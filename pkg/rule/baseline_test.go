package rule

import (
	"net/http"
	"strings"
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func TestBaselineIsAlwaysRegistered(t *testing.T) {
	t.Parallel()

	for _, r := range All() {
		if r.ID() == BaselineID {
			if !r.Applies(api.RouteSpec{}) {
				t.Error("the baseline rule must apply to every route")
			}
			return
		}
	}
	t.Fatalf("baseline rule is not registered; All() = %v", ids(All()))
}

func TestBaselineGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		route api.RouteSpec
		want  []string // "<scheme> <path>", in order
	}{
		{
			name:  "prefix root, plaintext",
			route: api.RouteSpec{ID: "shop/web/h/", Path: "/", PathType: api.PathTypePrefix},
			// "/" has no trailing slash to flip, so only two variants.
			want: []string{"http /", "http /swapsafe-probe"},
		},
		{
			name:  "prefix sub-path with TLS",
			route: api.RouteSpec{ID: "shop/web/h/shop", Path: "/shop", PathType: api.PathTypePrefix, TLS: true},
			want: []string{
				"http /shop", "http /shop/", "http /shop/swapsafe-probe",
				"https /shop", "https /shop/", "https /shop/swapsafe-probe",
			},
		},
		{
			name:  "exact path has no sub-path",
			route: api.RouteSpec{ID: "shop/web/h/checkout", Path: "/checkout", PathType: api.PathTypeExact},
			want:  []string{"http /checkout", "http /checkout/"},
		},
		{
			// The path is canonicalized first, so the probes come out in the
			// same order however the Ingress spelled it.
			name:  "trailing slash is flipped off",
			route: api.RouteSpec{ID: "shop/web/h/shop/", Path: "/shop/", PathType: api.PathTypePrefix},
			want:  []string{"http /shop", "http /shop/", "http /shop/swapsafe-probe"},
		},
		{
			name:  "a regex path is probed as the path part of it",
			route: api.RouteSpec{ID: "demo/x/h/podinfo", Path: "/podinfo(/|$)(.*)", PathType: api.PathTypeImplementationSpecific},
			want:  []string{"http /podinfo", "http /podinfo/", "http /podinfo/swapsafe-probe"},
		},
		{
			name:  "empty path is treated as root",
			route: api.RouteSpec{ID: "shop/web/h", PathType: api.PathTypeImplementationSpecific},
			want:  []string{"http /", "http /swapsafe-probe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cases := (baselineRule{}).Generate(tt.route)
			if len(cases) != len(tt.want) {
				t.Fatalf("got %d test cases, want %d: %v", len(cases), len(tt.want), variants(cases))
			}
			for i, want := range tt.want {
				if got := variant(cases[i]); got != want {
					t.Errorf("case %d = %q, want %q", i, got, want)
				}
			}
			for _, tc := range cases {
				if tc.Request.Method != http.MethodGet {
					t.Errorf("method = %q, want GET: baseline probes must be safe", tc.Request.Method)
				}
				if tc.RouteID != tt.route.ID || tc.RuleID != BaselineID {
					t.Errorf("case is not attributed to the route: %+v", tc)
				}
				if len(tc.Expect) == 0 {
					t.Error("case has no assertions, so the differ would ignore it")
				}
			}
		})
	}
}

// Fix the wanted path list above and this test is what catches a second case
// sharing an ID: distinct requests must never collapse into one test case.
func TestBaselineIDsAreUnique(t *testing.T) {
	t.Parallel()

	route := api.RouteSpec{ID: "shop/web/h/shop", Path: "/shop", PathType: api.PathTypePrefix, TLS: true}
	seen := map[string]string{}
	for _, tc := range (baselineRule{}).Generate(route) {
		if prev, dup := seen[tc.ID]; dup {
			t.Errorf("%q and %q share the ID %s", prev, variant(tc), tc.ID)
		}
		seen[tc.ID] = variant(tc)
	}
}

func TestBaselineGenerateIsDeterministic(t *testing.T) {
	t.Parallel()

	route := api.RouteSpec{ID: "shop/web/h/shop", Path: "/shop", PathType: api.PathTypePrefix, TLS: true}
	first := (baselineRule{}).Generate(route)
	for range 5 {
		next := (baselineRule{}).Generate(route)
		if len(next) != len(first) {
			t.Fatalf("got %d cases, want %d", len(next), len(first))
		}
		for i := range first {
			if next[i].ID != first[i].ID {
				t.Fatalf("case %d: ID changed between runs", i)
			}
		}
	}
}

func TestBaselineHint(t *testing.T) {
	t.Parallel()

	for _, aspect := range []string{"status", "header:location", "body"} {
		if got := (baselineRule{}).Hint(api.Finding{Aspect: aspect}); got == "" {
			t.Errorf("no hint for aspect %q", aspect)
		}
	}
	if got := (baselineRule{}).Hint(api.Finding{Aspect: "cors"}); got != "" {
		t.Errorf("unexpected hint for an aspect the baseline does not own: %q", got)
	}
}

func variant(tc api.TestCase) string {
	return tc.Request.Scheme + " " + tc.Request.Path
}

func variants(cases []api.TestCase) []string {
	out := make([]string, 0, len(cases))
	for _, tc := range cases {
		out = append(out, variant(tc))
	}
	return out
}

// The body hint must not claim a different backend when the baseline simply
// never answers the same thing twice.
func TestBaselineHintSeparatesUnstableBodies(t *testing.T) {
	t.Parallel()

	r := baselineRule{}
	unstable := r.Hint(api.Finding{Aspect: api.AspectBody, Severity: api.SeverityInfo})
	differs := r.Hint(api.Finding{Aspect: api.AspectBody, Severity: api.SeverityMajor})

	if unstable == differs {
		t.Fatal("the same hint is given whether or not the baseline is stable")
	}
	if strings.Contains(unstable, "different backend") {
		t.Errorf("an unstable body was blamed on the backend: %q", unstable)
	}
	if !strings.Contains(differs, "different backend") {
		t.Errorf("a real body difference lost its hint: %q", differs)
	}
}
