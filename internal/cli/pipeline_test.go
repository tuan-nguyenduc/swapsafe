package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tuan-nguyenduc/swapsafe/internal/attribution"
	"github.com/tuan-nguyenduc/swapsafe/internal/collector"
	"github.com/tuan-nguyenduc/swapsafe/internal/differ"
	"github.com/tuan-nguyenduc/swapsafe/internal/generator"
	"github.com/tuan-nguyenduc/swapsafe/internal/normalize"
	"github.com/tuan-nguyenduc/swapsafe/internal/prober"
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

const manifest = `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: shop
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    # No rule covers this one yet (it is on the v0.3 list), so it exercises
    # the "not verified" path that must never be silent.
    nginx.ingress.kubernetes.io/limit-rps: "5"
spec:
  rules:
    - host: shop.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: {service: {name: web, port: {number: 80}}}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: web, namespace: shop}
spec:
  parentRefs: [{name: gateway}]
  hostnames: [shop.example.com]
  rules:
    - matches: [{path: {type: PathPrefix, value: /}}]
      backendRefs: [{name: web, port: 80}]
`

// TestPipeline wires every stage together against two local servers that
// differ the way a forgotten redirect filter would. Until the run command
// exists (M1-T10), this is the only check that the stages fit together.
func TestPipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifests.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	// baseline = ingress-nginx with ssl-redirect: every plain HTTP request 301s.
	baseline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "nginx/1.25.3")
		w.Header().Set("Location", "https://shop.example.com"+r.URL.Path)
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer baseline.Close()

	// candidate = a Gateway where the redirect filter was forgotten.
	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "envoy")
		_, _ = w.Write([]byte("the real page"))
	}))
	defer candidate.Close()

	collected, err := collector.FromFiles(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	gen := generator.Generate(collected.Routes)

	p, err := prober.New(prober.Options{
		Baseline: baseline.URL, Candidate: candidate.URL, RatePerSecond: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	observations, err := p.Run(t.Context(), gen.TestCases)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]api.TestCase{}
	for _, tc := range gen.TestCases {
		byID[tc.ID] = tc
	}
	for id, list := range observations {
		for i, o := range list {
			list[i] = normalize.Observation(o, byID[id].Request, normalize.Options{})
		}
	}

	// slices.Concat, not append: appending onto collected.Findings could
	// write into the collector's own backing array.
	findings := slices.Concat(collected.Findings, gen.Findings,
		differ.DiffAll(gen.TestCases, observations))
	findings = attribution.Apply(findings, collected.Routes, gen.TestCases)

	t.Logf("%d routes, %d test cases, %d findings", len(collected.Routes), len(gen.TestCases), len(findings))
	for _, f := range findings {
		cause := ""
		if len(f.Attribution) > 0 {
			cause = f.Attribution[0].Annotation
			if cause == "" {
				cause = f.Attribution[0].Object.String()
			}
		}
		t.Logf("  [%-8s] %-22s %-18s base=%-28s cand=%-28s  <- %s",
			f.Severity, f.Aspect, f.RouteID, f.Baseline, f.Candidate, cause)
	}
	if len(findings) == 0 {
		t.Fatal("the pipeline found nothing on a route that obviously differs")
	}

	// The two servers announce different Server headers. That is the proxy
	// naming itself, not a behaviour difference, and normalize must eat it.
	for _, f := range findings {
		if f.Aspect == api.HeaderAspect("Server") {
			t.Errorf("the Server header leaked into the findings: %+v", f)
		}
	}

	// The real difference must be found, and must be critical.
	var critical int
	for _, f := range findings {
		if f.Severity == api.SeverityCritical && f.Aspect == api.AspectStatus {
			critical++
		}
	}
	if critical == 0 {
		t.Error("301 against 200 was not reported as a critical status difference")
	}
}
