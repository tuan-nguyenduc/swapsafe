package attribution

import (
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

const annotation = "nginx.ingress.kubernetes.io/ssl-redirect"

// sslRedirect is a stand-in for the real rule, which lands in M1-T9.
type sslRedirect struct{}

func (sslRedirect) ID() string                            { return "nginx.ssl-redirect" }
func (sslRedirect) Annotations() []string                 { return []string{annotation} }
func (sslRedirect) Applies(api.RouteSpec) bool            { return true }
func (sslRedirect) Generate(api.RouteSpec) []api.TestCase { return nil }
func (sslRedirect) Compare(api.TestCase, api.Observation, api.Observation) []api.Finding {
	return nil
}
func (sslRedirect) Hint(api.Finding) string { return "use a RequestRedirect filter" }

func registerSSLRedirect(t *testing.T) {
	t.Helper()
	for _, r := range rule.All() {
		if r.ID() == (sslRedirect{}).ID() {
			t.Skip("the real ssl-redirect rule now exists; this stand-in is obsolete")
		}
	}
	rule.Register(sslRedirect{})
}

func fixture() (api.RouteSpec, api.TestCase) {
	route := api.RouteSpec{
		ID:   "shop/web/shop.example.com/",
		Host: "shop.example.com",
		Annotations: map[string]string{
			annotation: "true",
			"nginx.ingress.kubernetes.io/rewrite-target": "/",
		},
		Source: api.ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"},
	}
	tc := api.TestCase{ID: "tc1", RouteID: route.ID, RuleID: "nginx.ssl-redirect"}
	return route, tc
}

func TestApplyAttributesToTheRulesAnnotation(t *testing.T) {
	registerSSLRedirect(t)
	route, tc := fixture()

	findings := []api.Finding{{
		RouteID:    route.ID,
		TestCaseID: tc.ID,
		Severity:   api.SeverityMajor,
		Aspect:     api.AspectStatus,
	}}

	got := Apply(findings, []api.RouteSpec{route}, []api.TestCase{tc})
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if len(got[0].Attribution) != 1 {
		t.Fatalf("got %d causes, want 1: %+v", len(got[0].Attribution), got[0].Attribution)
	}

	cause := got[0].Attribution[0]
	if cause.Annotation != annotation {
		t.Errorf("annotation = %q, want %q: only the rule's own annotations count", cause.Annotation, annotation)
	}
	if cause.Value != "true" {
		t.Errorf("value = %q, want %q", cause.Value, "true")
	}
	if cause.Object.Name != "web" {
		t.Errorf("cause does not point at the Ingress: %+v", cause.Object)
	}
	if got[0].Hint != "use a RequestRedirect filter" {
		t.Errorf("hint = %q, want the rule's own", got[0].Hint)
	}
}

// A rule driven by no annotation still has to name something the user can go
// and look at.
func TestApplyFallsBackToTheSourceObject(t *testing.T) {
	t.Parallel()

	route := api.RouteSpec{
		ID:     "shop/web/shop.example.com/",
		Source: api.ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"},
	}
	tc := api.TestCase{ID: "tc1", RouteID: route.ID, RuleID: rule.BaselineID}

	got := Apply([]api.Finding{{RouteID: route.ID, TestCaseID: tc.ID}},
		[]api.RouteSpec{route}, []api.TestCase{tc})

	if len(got[0].Attribution) != 1 {
		t.Fatalf("got %d causes, want 1", len(got[0].Attribution))
	}
	if got[0].Attribution[0].Annotation != "" {
		t.Errorf("the baseline rule invented an annotation: %+v", got[0].Attribution[0])
	}
	if got[0].Attribution[0].Object.Name != "web" {
		t.Errorf("cause does not point at the Ingress: %+v", got[0].Attribution[0].Object)
	}
}

// The collector and the generator already attribute their own findings.
func TestApplyLeavesExistingAttributionAlone(t *testing.T) {
	t.Parallel()

	route := api.RouteSpec{ID: "r1", Source: api.ObjectRef{Kind: "Ingress", Name: "web"}}
	existing := api.Finding{
		RouteID:     route.ID,
		Aspect:      api.AspectRoute,
		Attribution: []api.Cause{{Annotation: "already", Object: route.Source}},
		Hint:        "already written",
	}

	got := Apply([]api.Finding{existing}, []api.RouteSpec{route}, nil)
	if len(got[0].Attribution) != 1 || got[0].Attribution[0].Annotation != "already" {
		t.Errorf("existing attribution was overwritten: %+v", got[0].Attribution)
	}
	if got[0].Hint != "already written" {
		t.Errorf("existing hint was overwritten: %q", got[0].Hint)
	}
}

func TestApplyToleratesMissingReferences(t *testing.T) {
	t.Parallel()

	got := Apply([]api.Finding{{RouteID: "gone", TestCaseID: "gone"}}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want the finding kept", len(got))
	}
	if len(got[0].Attribution) != 0 {
		t.Errorf("attribution invented for an unknown route: %+v", got[0].Attribution)
	}
}
