package rule

import (
	"net/http"
	"testing"

	"github.com/tuan-nguyenduc/swapsafe/internal/model"
)

func TestBaselineIsAlwaysRegistered(t *testing.T) {
	t.Parallel()

	for _, r := range All() {
		if r.ID() == BaselineID {
			if !r.Applies(model.RouteSpec{}) {
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
		route model.RouteSpec
		want  []string // "<scheme> <path>", in order
	}{
		{
			name:  "prefix root, plaintext",
			route: model.RouteSpec{ID: "shop/web/h/", Path: "/", PathType: model.PathTypePrefix},
			// "/" has no trailing slash to flip, so only two variants.
			want: []string{"http /", "http /swapsafe-probe"},
		},
		{
			name:  "prefix sub-path with TLS",
			route: model.RouteSpec{ID: "shop/web/h/shop", Path: "/shop", PathType: model.PathTypePrefix, TLS: true},
			want: []string{
				"http /shop", "http /shop/", "http /shop/swapsafe-probe",
				"https /shop", "https /shop/", "https /shop/swapsafe-probe",
			},
		},
		{
			name:  "exact path has no sub-path",
			route: model.RouteSpec{ID: "shop/web/h/checkout", Path: "/checkout", PathType: model.PathTypeExact},
			want:  []string{"http /checkout", "http /checkout/"},
		},
		{
			name:  "trailing slash is flipped off",
			route: model.RouteSpec{ID: "shop/web/h/shop/", Path: "/shop/", PathType: model.PathTypePrefix},
			want:  []string{"http /shop/", "http /shop", "http /shop/swapsafe-probe"},
		},
		{
			name:  "empty path is treated as root",
			route: model.RouteSpec{ID: "shop/web/h", PathType: model.PathTypeImplementationSpecific},
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

	route := model.RouteSpec{ID: "shop/web/h/shop", Path: "/shop", PathType: model.PathTypePrefix, TLS: true}
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

	route := model.RouteSpec{ID: "shop/web/h/shop", Path: "/shop", PathType: model.PathTypePrefix, TLS: true}
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
		if got := (baselineRule{}).Hint(model.Finding{Aspect: aspect}); got == "" {
			t.Errorf("no hint for aspect %q", aspect)
		}
	}
	if got := (baselineRule{}).Hint(model.Finding{Aspect: "cors"}); got != "" {
		t.Errorf("unexpected hint for an aspect the baseline does not own: %q", got)
	}
}

func variant(tc model.TestCase) string {
	return tc.Request.Scheme + " " + tc.Request.Path
}

func variants(cases []model.TestCase) []string {
	out := make([]string, 0, len(cases))
	for _, tc := range cases {
		out = append(out, variant(tc))
	}
	return out
}
