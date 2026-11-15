package collector

import (
	"context"
	"errors"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func ingress(name, namespace, host string, class *string, labels map[string]string, annotations map[string]string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: namespace, Labels: labels, Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: class,
			Rules: []networkingv1.IngressRule{{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &pathType}},
					},
				},
			}},
		},
	}
}

func httpRoute(name, namespace, host string) *gatewayv1.HTTPRoute {
	prefix := gatewayv1.PathMatchPathPrefix
	value := "/"
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(host)},
			Rules: []gatewayv1.HTTPRouteRule{{
				Matches: []gatewayv1.HTTPRouteMatch{{
					Path: &gatewayv1.HTTPPathMatch{Type: &prefix, Value: &value},
				}},
			}},
		},
	}
}

func ptr(s string) *string { return &s }

func TestFromClientsPairsAcrossNamespaces(t *testing.T) {
	t.Parallel()

	core := corefake.NewSimpleClientset(
		ingress("web", "shop", "shop.example.com", ptr("nginx"), nil, nil),
		ingress("web", "blog", "blog.example.com", ptr("nginx"), nil, nil),
	)
	gateway := gatewayfake.NewSimpleClientset(
		httpRoute("web", "shop", "shop.example.com"),
	)

	got, err := fromClients(t.Context(), core, gateway, ClusterOptions{})
	if err != nil {
		t.Fatalf("fromClients: %v", err)
	}

	if len(got.Routes) != 2 {
		t.Fatalf("got %d routes, want 2: %+v", len(got.Routes), got.Routes)
	}
	// Sorted by ID: blog before shop.
	if got.Routes[0].Target != nil {
		t.Errorf("blog has no HTTPRoute but was paired: %+v", got.Routes[0].Target)
	}
	if got.Routes[1].Target == nil || got.Routes[1].Target.Name != "web" {
		t.Errorf("shop was not paired: %+v", got.Routes[1].Target)
	}
	if len(got.Findings) != 1 {
		t.Errorf("got %d findings, want 1 for the unpaired blog route", len(got.Findings))
	}
}

func TestFromClientsFiltersByIngressClass(t *testing.T) {
	t.Parallel()

	core := corefake.NewSimpleClientset(
		ingress("modern", "shop", "a.example.com", ptr("nginx"), nil, nil),
		ingress("legacy", "shop", "b.example.com", nil, nil,
			map[string]string{LegacyIngressClassAnnotation: "nginx"}),
		ingress("traefik", "shop", "c.example.com", ptr("traefik"), nil, nil),
		ingress("classless", "shop", "d.example.com", nil, nil, nil),
	)
	gateway := gatewayfake.NewSimpleClientset()

	t.Run("named class keeps both spellings", func(t *testing.T) {
		got, err := fromClients(t.Context(), core, gateway, ClusterOptions{IngressClass: "nginx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Routes) != 2 {
			t.Errorf("got %d routes, want the modern and the legacy nginx one: %v", len(got.Routes), ids(got.Routes))
		}
	})

	t.Run("empty class keeps everything", func(t *testing.T) {
		got, err := fromClients(t.Context(), core, gateway, ClusterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Routes) != 4 {
			t.Errorf("got %d routes, want all 4: %v", len(got.Routes), ids(got.Routes))
		}
	})
}

// The namespace and the selector are handed to the API server, not applied
// here: the point is to list less, not to filter after listing.
func TestFromClientsPassesTheSelectionToTheServer(t *testing.T) {
	t.Parallel()

	core := corefake.NewSimpleClientset()
	var sawNamespace, sawSelector string
	core.PrependReactor("list", "ingresses", func(action k8stesting.Action) (bool, runtime.Object, error) {
		sawNamespace = action.GetNamespace()
		sawSelector = action.(k8stesting.ListAction).GetListRestrictions().Labels.String()
		return true, &networkingv1.IngressList{}, nil
	})

	_, err := fromClients(t.Context(), core, gatewayfake.NewSimpleClientset(), ClusterOptions{
		Namespace: "shop",
		Selector:  "app=web",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sawNamespace != "shop" {
		t.Errorf("listed namespace %q, want shop", sawNamespace)
	}
	if sawSelector != "app=web" {
		t.Errorf("listed with selector %q, want app=web", sawSelector)
	}
}

func TestFromClientsReportsListErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("forbidden: swapsafe needs list on ingresses")
	core := corefake.NewSimpleClientset()
	core.PrependReactor("list", "ingresses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, wantErr
	})

	_, err := fromClients(t.Context(), core, gatewayfake.NewSimpleClientset(), ClusterOptions{})
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap the API error", err)
	}
}

// Gateway API may simply not be installed. That has to read as a clear error,
// not as "no routes found".
func TestFromClientsReportsMissingGatewayAPI(t *testing.T) {
	t.Parallel()

	wantErr := errors.New(`the server could not find the requested resource (get httproutes.gateway.networking.k8s.io)`)
	gateway := gatewayfake.NewSimpleClientset()
	gateway.PrependReactor("list", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, wantErr
	})

	_, err := fromClients(t.Context(), corefake.NewSimpleClientset(), gateway, ClusterOptions{})
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap the API error", err)
	}
}

func TestFromClientsRespectsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	core := corefake.NewSimpleClientset()
	core.PrependReactor("list", "ingresses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, ctx.Err()
	})

	if _, err := fromClients(ctx, core, gatewayfake.NewSimpleClientset(), ClusterOptions{}); err == nil {
		t.Error("a cancelled context was ignored")
	}
}

func TestClusterOptionsDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		opts ClusterOptions
		want string
	}{
		{want: "namespace=(all)"},
		{opts: ClusterOptions{Namespace: "shop"}, want: "namespace=shop"},
		{
			opts: ClusterOptions{Namespace: "shop", Selector: "app=web", IngressClass: "nginx"},
			want: "namespace=shop selector=app=web ingressClass=nginx",
		},
	}

	for _, tt := range tests {
		if got := tt.opts.Describe(); got != tt.want {
			t.Errorf("Describe() = %q, want %q", got, tt.want)
		}
	}
}

func ids(routes []api.RouteSpec) []string {
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		out = append(out, r.ID)
	}
	return out
}
