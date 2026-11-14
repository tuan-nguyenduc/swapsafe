package collector

import (
	"context"
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func TestFromFiles(t *testing.T) {
	t.Parallel()

	got, err := FromFiles(t.Context(), "testdata/manifests.yaml")
	if err != nil {
		t.Fatalf("FromFiles: %v", err)
	}

	wantIDs := []string{
		"shop/admin/admin.example.com/",
		"shop/web/shop.example.com/",
		"shop/web/shop.example.com/checkout",
	}
	if len(got.Routes) != len(wantIDs) {
		t.Fatalf("got %d routes, want %d: %+v", len(got.Routes), len(wantIDs), got.Routes)
	}
	for i, want := range wantIDs {
		if got.Routes[i].ID != want {
			t.Errorf("route %d ID = %q, want %q (routes must be sorted by ID)", i, got.Routes[i].ID, want)
		}
	}

	admin, web, checkout := got.Routes[0], got.Routes[1], got.Routes[2]

	if web.Target == nil || web.Target.Name != "web" || web.Target.Kind != "HTTPRoute" {
		t.Errorf("shop/web was not paired: %+v", web.Target)
	}
	if !web.TLS {
		t.Error("shop.example.com is in spec.tls, so TLS must be true")
	}
	if web.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] != "true" {
		t.Errorf("annotations lost: %+v", web.Annotations)
	}
	if checkout.PathType != api.PathTypeExact {
		t.Errorf("pathType = %q, want Exact", checkout.PathType)
	}
	if checkout.Target == nil {
		t.Error("the Exact /checkout rule was not paired")
	}

	// The only HTTPRoute for admin.example.com lives in another namespace.
	if admin.Target != nil {
		t.Errorf("shop/admin paired across namespaces: %+v", admin.Target)
	}
	if admin.TLS {
		t.Error("admin has no spec.tls, so TLS must be false")
	}

	if len(got.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got.Findings), got.Findings)
	}
	f := got.Findings[0]
	if f.RouteID != admin.ID {
		t.Errorf("finding RouteID = %q, want %q", f.RouteID, admin.ID)
	}
	if f.Severity != api.SeverityCritical {
		t.Errorf("severity = %q, want critical", f.Severity)
	}
	if f.TestCaseID != "" {
		t.Errorf("a route finding must carry no test case, got %q", f.TestCaseID)
	}
	if f.Aspect != AspectRoute {
		t.Errorf("aspect = %q, want %q", f.Aspect, AspectRoute)
	}
	if len(f.Attribution) != 1 || f.Attribution[0].Object.Name != "admin" {
		t.Errorf("finding does not point at the Ingress: %+v", f.Attribution)
	}
	if f.Hint == "" {
		t.Error("finding has no hint")
	}
}

// Passing one combined manifest as both --ingress-file and --httproute-file
// must not read it twice.
func TestFromFilesDeduplicatesPaths(t *testing.T) {
	t.Parallel()

	once, err := FromFiles(t.Context(), "testdata/manifests.yaml")
	if err != nil {
		t.Fatalf("FromFiles: %v", err)
	}
	twice, err := FromFiles(t.Context(), "testdata/manifests.yaml", "./testdata/manifests.yaml")
	if err != nil {
		t.Fatalf("FromFiles: %v", err)
	}
	if len(twice.Routes) != len(once.Routes) {
		t.Errorf("got %d routes from the same file twice, want %d", len(twice.Routes), len(once.Routes))
	}
}

func TestFromFilesErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		if _, err := FromFiles(t.Context(), "testdata/does-not-exist.yaml"); err == nil {
			t.Fatal("want an error for a missing file")
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := FromFiles(t.Context(), "testdata/invalid.yaml")
		if err == nil {
			t.Fatal("want an error for a misspelled field")
		}
		if !strings.Contains(err.Error(), "invalid.yaml") {
			t.Errorf("error does not name the file: %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := FromFiles(ctx, "testdata/manifests.yaml"); err == nil {
			t.Fatal("want an error from a cancelled context")
		}
	})

	t.Run("empty path is skipped", func(t *testing.T) {
		t.Parallel()
		got, err := FromFiles(t.Context(), "")
		if err != nil {
			t.Fatalf("FromFiles(\"\"): %v", err)
		}
		if len(got.Routes) != 0 {
			t.Errorf("got %d routes, want none", len(got.Routes))
		}
	})
}

func TestHostMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host, pattern string
		want          bool
	}{
		{host: "a.example.com", pattern: "a.example.com", want: true},
		{host: "a.example.com", pattern: "b.example.com"},
		{host: "a.example.com", pattern: "", want: true},
		{host: "a.example.com", pattern: "*.example.com", want: true},
		{host: "a.b.example.com", pattern: "*.example.com"}, // wildcard is one label
		{host: "example.com", pattern: "*.example.com"},     // needs a label
		{host: "", pattern: "*.example.com"},
		{host: "", pattern: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"/"+tt.host, func(t *testing.T) {
			t.Parallel()
			if got := hostMatches(tt.host, tt.pattern); got != tt.want {
				t.Errorf("hostMatches(%q, %q) = %v, want %v", tt.host, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestPathMatches(t *testing.T) {
	t.Parallel()

	prefix := gatewayv1.PathMatchPathPrefix
	exact := gatewayv1.PathMatchExact
	regex := gatewayv1.PathMatchRegularExpression
	ptr := func(s string) *string { return &s }

	tests := []struct {
		name    string
		match   *gatewayv1.HTTPPathMatch
		ingPath string
		want    bool
	}{
		{name: "nil match covers everything", ingPath: "/anything", want: true},
		{name: "prefix equal", match: &gatewayv1.HTTPPathMatch{Type: &prefix, Value: ptr("/shop")}, ingPath: "/shop", want: true},
		{name: "trailing slash is ignored", match: &gatewayv1.HTTPPathMatch{Type: &prefix, Value: ptr("/shop/")}, ingPath: "/shop", want: true},
		{name: "root stays root", match: &gatewayv1.HTTPPathMatch{Type: &prefix, Value: ptr("/")}, ingPath: "/", want: true},
		{name: "different path", match: &gatewayv1.HTTPPathMatch{Type: &prefix, Value: ptr("/shop")}, ingPath: "/admin"},
		{name: "exact equal", match: &gatewayv1.HTTPPathMatch{Type: &exact, Value: ptr("/checkout")}, ingPath: "/checkout", want: true},
		{name: "regex only pairs literally", match: &gatewayv1.HTTPPathMatch{Type: &regex, Value: ptr("/shop/.*")}, ingPath: "/shop"},
		{name: "type defaults to prefix", match: &gatewayv1.HTTPPathMatch{Value: ptr("/shop")}, ingPath: "/shop", want: true},
		{name: "empty ingress path is root", match: &gatewayv1.HTTPPathMatch{Type: &prefix, Value: ptr("/")}, ingPath: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pathMatches(tt.match, api.RouteSpec{Path: tt.ingPath})
			if got != tt.want {
				t.Errorf("pathMatches(%+v, %q) = %v, want %v", tt.match, tt.ingPath, got, tt.want)
			}
		})
	}
}

func TestRulesCoverEmptyMatches(t *testing.T) {
	t.Parallel()

	// A rule with no matches defaults to a "/" prefix: it covers any path.
	rules := []gatewayv1.HTTPRouteRule{{}}
	if !rulesCover(rules, api.RouteSpec{Path: "/deep/path"}) {
		t.Error("a rule with no matches must cover every path")
	}
	if rulesCover(nil, api.RouteSpec{Path: "/"}) {
		t.Error("an HTTPRoute with no rules covers nothing")
	}
}

func TestRoutesFromIngressSkipsRulesWithoutHTTP(t *testing.T) {
	t.Parallel()

	ing := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "shop.example.com"}},
		},
	}
	if got := routesFromIngress(ing); len(got) != 0 {
		t.Errorf("got %d routes from a rule with no HTTP section, want 0", len(got))
	}
}

func TestRoutesFromIngressDefaultsPathType(t *testing.T) {
	t.Parallel()

	ing := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "shop"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "shop.example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{Path: "/x"}},
					},
				},
			}},
		},
	}
	got := routesFromIngress(ing)
	if len(got) != 1 {
		t.Fatalf("got %d routes, want 1", len(got))
	}
	if got[0].PathType != api.PathTypeImplementationSpecific {
		t.Errorf("pathType = %q, want ImplementationSpecific", got[0].PathType)
	}
}
