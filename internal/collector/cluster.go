package collector

import (
	"context"
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// LegacyIngressClassAnnotation is how the ingress class was named before
// spec.ingressClassName existed. Plenty of live clusters still use it.
const LegacyIngressClassAnnotation = "kubernetes.io/ingress.class"

// ClusterOptions selects what to read from a cluster. swapsafe only ever
// lists; it never creates, updates or deletes, so a kubeconfig with get and
// list on Ingress and HTTPRoute is enough.
type ClusterOptions struct {
	// Kubeconfig is a path; empty means the usual discovery (KUBECONFIG, then
	// ~/.kube/config).
	Kubeconfig string
	// Context picks a kubeconfig context; empty means its current one.
	Context string
	// Namespace limits the listing; empty means every namespace.
	Namespace string
	// Selector is a label selector applied to both kinds.
	Selector string
	// IngressClass keeps only Ingresses on that class, by
	// spec.ingressClassName or the legacy annotation. Empty keeps all.
	IngressClass string
}

// FromCluster reads Ingress and HTTPRoute objects from a cluster and pairs
// them, exactly as FromFiles does for manifests.
func FromCluster(ctx context.Context, opts ClusterOptions) (Result, error) {
	cfg, err := restConfig(opts)
	if err != nil {
		return Result{}, err
	}

	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Result{}, fmt.Errorf("building the Kubernetes client: %w", err)
	}
	gateway, err := gatewayclient.NewForConfig(cfg)
	if err != nil {
		return Result{}, fmt.Errorf("building the Gateway API client: %w", err)
	}

	return fromClients(ctx, core, gateway, opts)
}

// fromClients does the listing. It takes the client interfaces rather than
// concrete clients, so the tests use the generated fakes instead of standing
// up an API server.
func fromClients(ctx context.Context, core kubernetes.Interface, gateway gatewayclient.Interface, opts ClusterOptions) (Result, error) {
	lo := metav1.ListOptions{LabelSelector: opts.Selector}

	ingressList, err := core.NetworkingV1().Ingresses(opts.Namespace).List(ctx, lo)
	if err != nil {
		return Result{}, fmt.Errorf("listing Ingresses: %w", err)
	}
	routeList, err := gateway.GatewayV1().HTTPRoutes(opts.Namespace).List(ctx, lo)
	if err != nil {
		return Result{}, fmt.Errorf("listing HTTPRoutes: %w", err)
	}

	kept := make([]networkingv1.Ingress, 0, len(ingressList.Items))
	for _, ing := range ingressList.Items {
		if matchesClass(ing, opts.IngressClass) {
			kept = append(kept, ing)
		}
	}
	return pair(kept, routeList.Items), nil
}

// matchesClass keeps an Ingress on the wanted class. An empty class keeps
// everything, including Ingresses that name no class at all.
func matchesClass(ing networkingv1.Ingress, class string) bool {
	if class == "" {
		return true
	}
	if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName == class {
		return true
	}
	return ing.Annotations[LegacyIngressClassAnnotation] == class
}

// restConfig builds the client configuration from a kubeconfig.
func restConfig(opts ClusterOptions) (*rest.Config, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	if opts.Kubeconfig != "" {
		loading.ExplicitPath = opts.Kubeconfig
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loading,
		&clientcmd.ConfigOverrides{CurrentContext: opts.Context},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig: %w", err)
	}

	// Name swapsafe in the audit log, so a cluster admin seeing these reads
	// knows what made them.
	cfg.UserAgent = "swapsafe"
	return cfg, nil
}

// Describe renders the cluster selection for a log line.
func (o ClusterOptions) Describe() string {
	parts := []string{"namespace=" + orAll(o.Namespace)}
	if o.Selector != "" {
		parts = append(parts, "selector="+o.Selector)
	}
	if o.IngressClass != "" {
		parts = append(parts, "ingressClass="+o.IngressClass)
	}
	return strings.Join(parts, " ")
}

func orAll(ns string) string {
	if ns == "" {
		return "(all)"
	}
	return ns
}
