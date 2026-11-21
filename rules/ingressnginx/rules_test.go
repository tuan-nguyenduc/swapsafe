package ingressnginx

import (
	"net/http"
	"strings"
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

func route(annotations map[string]string) api.RouteSpec {
	return api.RouteSpec{
		ID:          "shop/web/shop.example.com/",
		Host:        "shop.example.com",
		Path:        "/",
		PathType:    api.PathTypePrefix,
		Annotations: annotations,
		Source:      api.ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"},
	}
}

// find returns a registered rule by ID.
func find(t *testing.T, id string) rule.Rule {
	t.Helper()
	for _, r := range rule.All() {
		if r.ID() == id {
			return r
		}
	}
	t.Fatalf("rule %q is not registered", id)
	return nil
}

// Every rule in this package must be reachable, name its annotations, and
// have something to say when it finds a difference.
func TestEveryRuleIsWellFormed(t *testing.T) {
	t.Parallel()

	ids := []string{
		"nginx.ssl-redirect", "nginx.force-ssl-redirect", "nginx.rewrite-target",
		"nginx.use-regex", "nginx.app-root", "nginx.permanent-redirect",
		"nginx.temporal-redirect", "nginx.enable-cors", "nginx.proxy-body-size",
		"nginx.backend-protocol",
	}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			r := find(t, id)

			if len(r.Annotations()) == 0 {
				t.Error("rule declares no annotation, so nothing can ever activate it")
			}
			for _, a := range r.Annotations() {
				if len(a) <= len(Prefix) || a[:len(Prefix)] != Prefix {
					t.Errorf("annotation %q is not an ingress-nginx key", a)
				}
			}
			if r.Hint(api.Finding{}) == "" {
				t.Error("rule has no hint, so a finding from it tells the user nothing")
			}
			if r.Applies(route(nil)) {
				t.Error("rule applies to a route carrying none of its annotations")
			}
			if !r.Applies(route(map[string]string{r.Annotations()[0]: "x"})) {
				t.Error("rule does not apply to a route carrying its own annotation")
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ruleID      string
		annotations map[string]string
		path        string
		want        []string // "METHOD scheme path"
	}{
		{
			name:        "ssl-redirect probes plain http",
			ruleID:      "nginx.ssl-redirect",
			annotations: map[string]string{AnnSSLRedirect: "true"},
			want:        []string{"GET http /"},
		},
		{
			name:        "ssl-redirect false probes nothing",
			ruleID:      "nginx.ssl-redirect",
			annotations: map[string]string{AnnSSLRedirect: "false"},
		},
		{
			name:        "app-root probes the site root, not the route path",
			ruleID:      "nginx.app-root",
			annotations: map[string]string{AnnAppRoot: "/app"},
			path:        "/deep",
			want:        []string{"GET http /"},
		},
		{
			name:        "app-root with no value probes nothing",
			ruleID:      "nginx.app-root",
			annotations: map[string]string{AnnAppRoot: ""},
		},
		{
			// Both the path that the rewrite turns into the backend root,
			// where a real page lives, and one level below it.
			name:        "rewrite-target probes the route and under it",
			ruleID:      "nginx.rewrite-target",
			annotations: map[string]string{AnnRewriteTarget: "/"},
			path:        "/shop",
			want:        []string{"GET http /shop", "GET http /shop/swapsafe-probe"},
		},
		{
			name:        "use-regex probes a concrete path",
			ruleID:      "nginx.use-regex",
			annotations: map[string]string{AnnUseRegex: "true"},
			path:        "/api/v1/(.*)",
			want:        []string{"GET http /api/v1/swapsafe-probe"},
		},
		{
			name:        "use-regex off probes nothing",
			ruleID:      "nginx.use-regex",
			annotations: map[string]string{AnnUseRegex: "false"},
		},
		{
			name:        "permanent-redirect probes the route itself",
			ruleID:      "nginx.permanent-redirect",
			annotations: map[string]string{AnnPermanentRedirect: "https://elsewhere.example.com"},
			want:        []string{"GET http /"},
		},
		{
			name:        "cors sends a preflight and a simple request",
			ruleID:      "nginx.enable-cors",
			annotations: map[string]string{AnnEnableCORS: "true"},
			want:        []string{"OPTIONS http /", "GET http /"},
		},
		{
			name:        "report-only rules send nothing",
			ruleID:      "nginx.proxy-body-size",
			annotations: map[string]string{AnnProxyBodySize: "8m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := route(tt.annotations)
			if tt.path != "" {
				r.Path = tt.path
			}
			cases := find(t, tt.ruleID).Generate(r)

			if len(cases) != len(tt.want) {
				t.Fatalf("got %d test cases, want %d: %v", len(cases), len(tt.want), describe(cases))
			}
			for i, want := range tt.want {
				got := cases[i].Request.Method + " " + cases[i].Request.Scheme + " " + cases[i].Request.Path
				if got != want {
					t.Errorf("case %d = %q, want %q", i, got, want)
				}
				if cases[i].RuleID != tt.ruleID {
					t.Errorf("case %d is attributed to %q", i, cases[i].RuleID)
				}
				if len(cases[i].Expect) == 0 {
					t.Errorf("case %d declares no aspects", i)
				}
			}
		})
	}
}

// A preflight is useless without the headers that make it one.
func TestCORSSendsARealPreflight(t *testing.T) {
	t.Parallel()

	cases := find(t, "nginx.enable-cors").Generate(route(map[string]string{AnnEnableCORS: "true"}))
	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2", len(cases))
	}

	preflight := cases[0].Request
	if preflight.Method != http.MethodOptions {
		t.Fatalf("first case is %s, want OPTIONS", preflight.Method)
	}
	for _, header := range []string{"Origin", "Access-Control-Request-Method"} {
		if preflight.Headers.Get(header) == "" {
			t.Errorf("preflight is missing %s", header)
		}
	}
	if cases[1].Request.Headers.Get("Origin") == "" {
		t.Error("the simple request carries no Origin, so CORS would never show")
	}
}

// Two test cases of the same rule must not collapse into one.
func TestCORSCaseIDsDiffer(t *testing.T) {
	t.Parallel()

	cases := find(t, "nginx.enable-cors").Generate(route(map[string]string{AnnEnableCORS: "true"}))
	if cases[0].ID == cases[1].ID {
		t.Error("the preflight and the simple request share an ID: the header is not in the hash")
	}
}

func TestReportOnlyRulesSpeakUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ruleID     string
		annotation string
		value      string
	}{
		{ruleID: "nginx.proxy-body-size", annotation: AnnProxyBodySize, value: "8m"},
		{ruleID: "nginx.backend-protocol", annotation: AnnBackendProtocol, value: "GRPC"},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			t.Parallel()

			reporter, ok := find(t, tt.ruleID).(rule.Reporter)
			if !ok {
				t.Fatalf("%s cannot probe and does not report: it would be silent", tt.ruleID)
			}

			got := reporter.Report(route(map[string]string{tt.annotation: tt.value}))
			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1", len(got))
			}
			if got[0].Severity == api.SeverityInfo {
				t.Error("an annotation swapsafe cannot verify at all is more than info")
			}
			if got[0].Hint == "" {
				t.Error("no hint, so the user is told there is a problem and nothing else")
			}
			if len(got[0].Attribution) != 1 || got[0].Attribution[0].Annotation != tt.annotation {
				t.Errorf("finding does not name its annotation: %+v", got[0].Attribution)
			}
			if len(reporter.Report(route(nil))) != 0 {
				t.Error("rule reports on a route that does not carry its annotation")
			}
		})
	}
}

// Every rule probes through routePath, so none of them can put a regular
// expression in a URL. The e2e suite caught rewrite-target doing exactly that
// after baseline had already been fixed.
func TestNoRuleProbesARegex(t *testing.T) {
	t.Parallel()

	regexRoute := route(map[string]string{
		AnnUseRegex:      "true",
		AnnRewriteTarget: "/$2",
		AnnSSLRedirect:   "true",
		AnnEnableCORS:    "true",
		AnnAppRoot:       "/app",
	})
	regexRoute.Path = "/podinfo(/|$)(.*)"

	for _, r := range rule.All() {
		for _, tc := range r.Generate(regexRoute) {
			if strings.ContainsAny(tc.Request.Path, "()|$*[]") {
				t.Errorf("%s probes %q, which is a pattern and not a path", r.ID(), tc.Request.Path)
			}
			if !strings.HasPrefix(tc.Request.Path, "/podinfo") && tc.Request.Path != "/" {
				t.Errorf("%s probes %q, which is outside the route", r.ID(), tc.Request.Path)
			}
		}
	}
}

func TestRoutePathAndSubPath(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, wantPath, wantSub string }{
		{in: "/api/v1/(.*)", wantPath: "/api/v1", wantSub: "/api/v1/swapsafe-probe"},
		{in: "/podinfo(/|$)(.*)", wantPath: "/podinfo", wantSub: "/podinfo/swapsafe-probe"},
		{in: "/shop", wantPath: "/shop", wantSub: "/shop/swapsafe-probe"},
		{in: "/shop/", wantPath: "/shop", wantSub: "/shop/swapsafe-probe"},
		{in: "/", wantPath: "/", wantSub: "/swapsafe-probe"},
		{in: "", wantPath: "/", wantSub: "/swapsafe-probe"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			r := api.RouteSpec{Path: tt.in}
			if got := routePath(r); got != tt.wantPath {
				t.Errorf("routePath(%q) = %q, want %q", tt.in, got, tt.wantPath)
			}
			if got := subPath(r); got != tt.wantSub {
				t.Errorf("subPath(%q) = %q, want %q", tt.in, got, tt.wantSub)
			}
		})
	}
}

func describe(cases []api.TestCase) []string {
	out := make([]string, 0, len(cases))
	for _, tc := range cases {
		out = append(out, tc.Request.Method+" "+tc.Request.Scheme+" "+tc.Request.Path)
	}
	return out
}
