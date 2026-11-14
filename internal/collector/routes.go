package collector

import (
	"strings"

	networkingv1 "k8s.io/api/networking/v1"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// routesFromIngress expands one Ingress into one RouteSpec per host/path pair.
func routesFromIngress(ing networkingv1.Ingress) []api.RouteSpec {
	source := api.ObjectRef{
		Group:     networkingv1.GroupName,
		Kind:      "Ingress",
		Namespace: ing.Namespace,
		Name:      ing.Name,
	}
	tlsHosts := tlsHostsOf(ing)

	var routes []api.RouteSpec
	for _, rule := range ing.Spec.Rules {
		// ponytail: a default-backend-only Ingress (no rules, or a rule with no
		// HTTP paths) produces no route, so swapsafe says nothing about it.
		// Give it a catch-all RouteSpec once we know how the Gateway side
		// models one.
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			pathType := api.PathTypeImplementationSpecific
			if p.PathType != nil {
				pathType = string(*p.PathType)
			}
			routes = append(routes, api.RouteSpec{
				ID:          routeID(ing.Namespace, ing.Name, rule.Host, p.Path),
				Host:        rule.Host,
				Path:        p.Path,
				PathType:    pathType,
				TLS:         hostInAny(rule.Host, tlsHosts),
				Annotations: ing.Annotations,
				Source:      source,
			})
		}
	}
	return routes
}

// routeID is "<namespace>/<ingress>/<host><path>" (PLAN section 5.1): stable
// across runs, so reports from different runs line up.
func routeID(namespace, name, host, path string) string {
	return namespace + "/" + name + "/" + host + path
}

func tlsHostsOf(ing networkingv1.Ingress) []string {
	var hosts []string
	for _, tls := range ing.Spec.TLS {
		hosts = append(hosts, tls.Hosts...)
	}
	return hosts
}

func hostInAny(host string, patterns []string) bool {
	for _, p := range patterns {
		if hostMatches(host, p) {
			return true
		}
	}
	return false
}

// hostMatches reports whether host is covered by pattern. An empty pattern is
// a catch-all; "*.example.com" covers any single label under example.com,
// which is how both Ingress and Gateway API read a wildcard hostname.
func hostMatches(host, pattern string) bool {
	switch {
	case pattern == "":
		return true
	case pattern == host:
		return true
	case strings.HasPrefix(pattern, "*."):
		suffix := pattern[1:] // ".example.com"
		label, ok := strings.CutSuffix(host, suffix)
		return ok && label != "" && !strings.Contains(label, ".")
	default:
		return false
	}
}
