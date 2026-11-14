package cli

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tuan-nguyenduc/swapsafe/internal/attribution"
	"github.com/tuan-nguyenduc/swapsafe/internal/differ"
	"github.com/tuan-nguyenduc/swapsafe/internal/generator"
	"github.com/tuan-nguyenduc/swapsafe/internal/normalize"
	"github.com/tuan-nguyenduc/swapsafe/internal/prober"
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	ingressnginx "github.com/tuan-nguyenduc/swapsafe/rules/ingressnginx"
)

// PLAN section 6.2: every rule needs at least one case where the two
// endpoints agree and one where they differ. Each scenario below supplies the
// differing pair; the matching case is the same scenario run with one server
// playing both parts.

type scenario struct {
	name        string
	ruleID      string
	annotations map[string]string
	path        string
	baseline    http.HandlerFunc
	candidate   http.HandlerFunc
	wantAspects []string
}

func redirects(status int, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", location)
		w.WriteHeader(status)
	}
}

func serves(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

func withCORS(origin, methods string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", methods)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func scenarios() []scenario {
	return []scenario{
		{
			name:        "ssl-redirect: the Gateway forgot to redirect",
			ruleID:      "nginx.ssl-redirect",
			annotations: map[string]string{ingressnginx.AnnSSLRedirect: "true"},
			baseline:    redirects(308, "https://shop.example.com/"),
			candidate:   serves("the real page"),
			wantAspects: []string{api.AspectStatus, api.HeaderAspect("Location")},
		},
		{
			name:        "force-ssl-redirect: same, without a TLS block",
			ruleID:      "nginx.force-ssl-redirect",
			annotations: map[string]string{ingressnginx.AnnForceSSLRedirect: "true"},
			baseline:    redirects(308, "https://shop.example.com/"),
			candidate:   serves("the real page"),
			wantAspects: []string{api.AspectStatus},
		},
		{
			name:        "rewrite-target: the sub-path reaches a different backend",
			ruleID:      "nginx.rewrite-target",
			annotations: map[string]string{ingressnginx.AnnRewriteTarget: "/"},
			path:        "/shop",
			baseline:    serves("backend saw /"),
			candidate:   serves("backend saw /shop/swapsafe-probe"),
			wantAspects: []string{api.AspectBody},
		},
		{
			name:        "use-regex: the Gateway does not match the pattern",
			ruleID:      "nginx.use-regex",
			annotations: map[string]string{ingressnginx.AnnUseRegex: "true"},
			path:        "/api/v1/(.*)",
			baseline:    serves("matched"),
			candidate:   func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantAspects: []string{api.AspectStatus},
		},
		{
			name:        "app-root: the redirect from / was lost",
			ruleID:      "nginx.app-root",
			annotations: map[string]string{ingressnginx.AnnAppRoot: "/app"},
			baseline:    redirects(302, "/app"),
			candidate:   serves("index"),
			wantAspects: []string{api.AspectStatus, api.HeaderAspect("Location")},
		},
		{
			name:        "permanent-redirect: 301 became 302",
			ruleID:      "nginx.permanent-redirect",
			annotations: map[string]string{ingressnginx.AnnPermanentRedirect: "https://new.example.com/"},
			baseline:    redirects(301, "https://new.example.com/"),
			candidate:   redirects(302, "https://new.example.com/"),
			wantAspects: []string{api.AspectStatus},
		},
		{
			name:        "temporal-redirect: the target changed",
			ruleID:      "nginx.temporal-redirect",
			annotations: map[string]string{ingressnginx.AnnTemporalRedirect: "https://new.example.com/"},
			baseline:    redirects(302, "https://new.example.com/"),
			candidate:   redirects(302, "https://other.example.com/"),
			wantAspects: []string{api.HeaderAspect("Location")},
		},
		{
			name:        "enable-cors: the Gateway answers without CORS headers",
			ruleID:      "nginx.enable-cors",
			annotations: map[string]string{ingressnginx.AnnEnableCORS: "true"},
			baseline:    withCORS("*", "GET, PUT, POST, OPTIONS"),
			candidate:   func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
			wantAspects: []string{api.HeaderAspect("Access-Control-Allow-Origin")},
		},
	}
}

func TestRulesFindTheDifference(t *testing.T) {
	t.Parallel()

	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()

			findings := runScenario(t, sc, sc.baseline, sc.candidate)

			var aspects []string
			for _, f := range findings {
				if f.TestCaseID != "" {
					aspects = append(aspects, f.Aspect)
				}
			}
			for _, want := range sc.wantAspects {
				if !slices.Contains(aspects, want) {
					t.Errorf("aspect %q was not reported; got %v", want, aspects)
				}
			}

			// The rule under test has to be the one explaining it, or the
			// user gets the baseline rule's generic hint instead.
			if !slices.ContainsFunc(findings, func(f api.Finding) bool {
				return strings.Contains(f.Hint, hintMarker(t, sc.ruleID))
			}) {
				t.Errorf("no finding carries the hint of %s", sc.ruleID)
			}
		})
	}
}

// The matching case: one server plays both endpoints, so the two answers are
// identical by construction and nothing probe-based may be reported.
func TestRulesAreQuietWhenBothSidesAgree(t *testing.T) {
	t.Parallel()

	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()

			for _, f := range runScenario(t, sc, sc.baseline, sc.baseline) {
				if f.TestCaseID != "" {
					t.Errorf("identical endpoints produced a finding: %+v", f)
				}
			}
		})
	}
}

// runScenario drives generator, prober, normalize, differ and attribution for
// one route, against two handlers.
func runScenario(t *testing.T, sc scenario, baseHandler, candHandler http.HandlerFunc) []api.Finding {
	t.Helper()

	baseline := httptest.NewServer(baseHandler)
	t.Cleanup(baseline.Close)
	candidate := httptest.NewServer(candHandler)
	t.Cleanup(candidate.Close)

	path := sc.path
	if path == "" {
		path = "/"
	}
	route := api.RouteSpec{
		ID:          "shop/web/shop.example.com" + path,
		Host:        "shop.example.com",
		Path:        path,
		PathType:    api.PathTypePrefix,
		Annotations: sc.annotations,
		Source:      api.ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"},
	}

	gen := generator.Generate([]api.RouteSpec{route})
	if !slices.ContainsFunc(gen.TestCases, func(tc api.TestCase) bool { return tc.RuleID == sc.ruleID }) {
		t.Fatalf("%s generated no test case for this route", sc.ruleID)
	}

	p, err := prober.New(prober.Options{
		Baseline: baseline.URL, Candidate: candidate.URL, RatePerSecond: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)

	observations, err := p.Run(t.Context(), gen.TestCases)
	if err != nil {
		t.Fatal(err)
	}

	byID := make(map[string]api.TestCase, len(gen.TestCases))
	for _, tc := range gen.TestCases {
		byID[tc.ID] = tc
	}
	for id, list := range observations {
		for i, o := range list {
			list[i] = normalize.Observation(o, byID[id].Request, normalize.Options{})
		}
	}

	// Only the rule under test, so one rule's findings cannot be credited to
	// another. The baseline rule probes the same paths and would drown it.
	var own []api.TestCase
	for _, tc := range gen.TestCases {
		if tc.RuleID == sc.ruleID {
			own = append(own, tc)
		}
	}

	findings := slices.Concat(gen.Findings, differ.DiffAll(own, observations))
	return attribution.Apply(findings, []api.RouteSpec{route}, gen.TestCases)
}

// hintMarker returns a distinctive slice of a rule's hint, to tell whose
// explanation ended up on a finding.
func hintMarker(t *testing.T, ruleID string) string {
	t.Helper()
	switch ruleID {
	case "nginx.ssl-redirect":
		return "answers 308 by default"
	case "nginx.force-ssl-redirect":
		return "port 80 listener"
	case "nginx.rewrite-target":
		return "URLRewrite filter"
	case "nginx.use-regex":
		return "RegularExpression is optional"
	case "nginx.app-root":
		return "ReplaceFullPath"
	case "nginx.permanent-redirect", "nginx.temporal-redirect":
		return "statusCode"
	case "nginx.enable-cors":
		return "CORS filter"
	default:
		t.Fatalf("no hint marker for %s", ruleID)
		return ""
	}
}
