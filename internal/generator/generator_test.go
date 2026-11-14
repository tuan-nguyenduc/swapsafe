package generator

import (
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

func route(id, path string, annotations map[string]string) api.RouteSpec {
	return api.RouteSpec{
		ID:          id,
		Host:        "shop.example.com",
		Path:        path,
		PathType:    api.PathTypePrefix,
		Annotations: annotations,
		Source:      api.ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"},
	}
}

func TestGenerateCoversEveryRoute(t *testing.T) {
	t.Parallel()

	routes := []api.RouteSpec{
		route("shop/web/shop.example.com/", "/", nil),
		route("shop/web/shop.example.com/checkout", "/checkout", nil),
	}

	got := Generate(routes)
	if len(got.TestCases) == 0 {
		t.Fatal("no test cases generated")
	}

	perRoute := map[string]int{}
	for _, tc := range got.TestCases {
		perRoute[tc.RouteID]++
		if tc.RuleID == "" || tc.ID == "" {
			t.Errorf("test case is missing its identity: %+v", tc)
		}
	}
	for _, r := range routes {
		if perRoute[r.ID] == 0 {
			t.Errorf("route %q produced no test cases; the baseline rule must cover every route", r.ID)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	t.Parallel()

	routes := []api.RouteSpec{
		route("shop/web/shop.example.com/", "/", map[string]string{
			NginxPrefix + "rewrite-target":                     "/",
			NginxPrefix + "ssl-redirect":                       "true",
			"kubectl.kubernetes.io/last-applied-configuration": "{}",
		}),
	}

	first := Generate(routes)
	for range 5 {
		next := Generate(routes)
		if len(next.TestCases) != len(first.TestCases) || len(next.Findings) != len(first.Findings) {
			t.Fatal("output length changed between runs")
		}
		for i := range first.TestCases {
			if next.TestCases[i].ID != first.TestCases[i].ID {
				t.Fatalf("test case %d changed between runs", i)
			}
		}
		for i := range first.Findings {
			if next.Findings[i].Baseline != first.Findings[i].Baseline {
				t.Fatalf("finding %d changed order between runs: map iteration is leaking", i)
			}
		}
	}
}

func TestGenerateDeduplicatesIDs(t *testing.T) {
	t.Parallel()

	// The same route twice: every test case ID repeats, and none may survive
	// a second time.
	r := route("shop/web/shop.example.com/", "/", nil)
	once := Generate([]api.RouteSpec{r})
	twice := Generate([]api.RouteSpec{r, r})

	if len(twice.TestCases) != len(once.TestCases) {
		t.Errorf("got %d test cases for a duplicated route, want %d", len(twice.TestCases), len(once.TestCases))
	}
}

func TestUnverifiedAnnotations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		annotations map[string]string
		want        []string // finding Baseline values, in order
	}{
		{
			name:        "no annotations",
			annotations: nil,
		},
		{
			name: "non-nginx annotations are not our business",
			annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
				"meta.helm.sh/release-name":                        "shop",
				"kubernetes.io/ingress.class":                      "nginx",
			},
		},
		{
			name: "nginx annotations are reported in sorted order",
			annotations: map[string]string{
				NginxPrefix + "ssl-redirect":    "true",
				NginxPrefix + "proxy-body-size": "8m",
				NginxPrefix + "rewrite-target":  "/$1",
			},
			want: []string{
				NginxPrefix + "proxy-body-size: 8m",
				NginxPrefix + "rewrite-target: /$1",
				NginxPrefix + "ssl-redirect: true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Generate([]api.RouteSpec{route("shop/web/h/", "/", tt.annotations)})
			if len(got.Findings) != len(tt.want) {
				t.Fatalf("got %d findings, want %d: %+v", len(got.Findings), len(tt.want), got.Findings)
			}
			for i, want := range tt.want {
				f := got.Findings[i]
				if f.Baseline != want {
					t.Errorf("finding %d = %q, want %q", i, f.Baseline, want)
				}
				if f.Severity != api.SeverityInfo {
					t.Errorf("finding %d severity = %q, want info", i, f.Severity)
				}
				if f.Aspect != api.AspectAnnotation {
					t.Errorf("finding %d aspect = %q, want %q", i, f.Aspect, api.AspectAnnotation)
				}
				if f.TestCaseID != "" {
					t.Errorf("finding %d has a test case, but nothing was probed for it", i)
				}
				if len(f.Attribution) != 1 || f.Attribution[0].Object.Name != "web" {
					t.Errorf("finding %d does not point at the Ingress: %+v", i, f.Attribution)
				}
			}
		})
	}
}

// An annotation a rule does cover must not also be reported as unverified.
func TestVerifiedAnnotationIsNotReported(t *testing.T) {
	t.Parallel()

	const covered = NginxPrefix + "ssl-redirect"
	if rule.Verifies(covered) {
		t.Skip("a rule now covers ssl-redirect; this test needs a different key")
	}

	got := Generate([]api.RouteSpec{route("shop/web/h/", "/", map[string]string{covered: "true"})})
	if len(got.Findings) != 1 {
		t.Fatalf("got %d findings, want 1 while no rule covers %s", len(got.Findings), covered)
	}
}
