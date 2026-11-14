package collector

import (
	"context"
	"sort"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/tuan-nguyenduc/swapsafe/internal/model"
)

// AspectRoute is the finding aspect used when a route has no counterpart on
// the Gateway side. It is not a response difference, so it carries no test case.
const AspectRoute = "route"

// Result is what the collector hands to the rest of the pipeline.
type Result struct {
	// Routes are sorted by ID, so every stage downstream is deterministic.
	Routes []model.RouteSpec
	// Findings reports routes with no matching HTTPRoute. Those are critical:
	// the traffic has nowhere to go after the cutover.
	Findings []model.Finding
}

// FromFiles reads Ingress and HTTPRoute objects from YAML manifests and pairs
// them. Each file may hold many documents; other kinds are ignored. Paths
// pointing at the same file are read once.
func FromFiles(ctx context.Context, paths ...string) (Result, error) {
	m, err := readFiles(ctx, paths...)
	if err != nil {
		return Result{}, err
	}
	return pair(m.ingresses, m.httpRoutes), nil
}

func pair(ingresses []networkingv1.Ingress, httpRoutes []gatewayv1.HTTPRoute) Result {
	var res Result

	for _, ing := range ingresses {
		for _, route := range routesFromIngress(ing) {
			if target, ok := findTarget(route, httpRoutes); ok {
				route.Target = &target
			} else {
				res.Findings = append(res.Findings, unmatchedFinding(route))
			}
			res.Routes = append(res.Routes, route)
		}
	}

	sort.Slice(res.Routes, func(i, j int) bool { return res.Routes[i].ID < res.Routes[j].ID })
	sort.Slice(res.Findings, func(i, j int) bool { return res.Findings[i].RouteID < res.Findings[j].RouteID })
	return res
}

// findTarget looks for the HTTPRoute that should carry this route's traffic:
// same namespace, a hostname covering the route's host, and a path match on
// the route's path.
func findTarget(route model.RouteSpec, httpRoutes []gatewayv1.HTTPRoute) (model.ObjectRef, bool) {
	for _, hr := range httpRoutes {
		if hr.Namespace != route.Source.Namespace {
			continue
		}
		if !hostnamesCover(hr.Spec.Hostnames, route.Host) {
			continue
		}
		if !rulesCover(hr.Spec.Rules, route) {
			continue
		}
		return model.ObjectRef{
			Group:     gatewayv1.GroupName,
			Kind:      "HTTPRoute",
			Namespace: hr.Namespace,
			Name:      hr.Name,
		}, true
	}
	return model.ObjectRef{}, false
}

// hostnamesCover reports whether an HTTPRoute serves host. An HTTPRoute with
// no hostnames inherits its listener's, so it can serve anything.
func hostnamesCover(hostnames []gatewayv1.Hostname, host string) bool {
	if len(hostnames) == 0 {
		return true
	}
	for _, h := range hostnames {
		if hostMatches(host, string(h)) {
			return true
		}
	}
	return false
}

// rulesCover reports whether any rule of an HTTPRoute matches the route's
// path. A rule with no matches defaults to a "/" prefix, which covers
// everything.
func rulesCover(rules []gatewayv1.HTTPRouteRule, route model.RouteSpec) bool {
	for _, rule := range rules {
		if len(rule.Matches) == 0 {
			return true
		}
		for _, m := range rule.Matches {
			if pathMatches(m.Path, route) {
				return true
			}
		}
	}
	return false
}

// pathMatches compares one HTTPRoute path match against the Ingress path.
// This is a static comparison of the configuration, not of behaviour: proving
// the two agree is what the probing pipeline does. Here we only need to decide
// which HTTPRoute is the counterpart of which Ingress route.
func pathMatches(p *gatewayv1.HTTPPathMatch, route model.RouteSpec) bool {
	if p == nil || p.Value == nil {
		// Defaults to PathPrefix "/", which covers every path.
		return true
	}
	value := normalizePath(*p.Value)
	want := normalizePath(route.Path)

	matchType := gatewayv1.PathMatchPathPrefix
	if p.Type != nil {
		matchType = *p.Type
	}

	switch matchType {
	case gatewayv1.PathMatchExact, gatewayv1.PathMatchPathPrefix:
		return value == want
	case gatewayv1.PathMatchRegularExpression:
		// ponytail: a regex path is only paired when it is written out
		// literally. Matching regex against an ingress path properly needs the
		// use-regex rule (M1-T9), which knows the nginx dialect.
		return value == want
	default:
		return false
	}
}

// normalizePath drops a trailing slash so "/shop" and "/shop/" pair up. The
// difference between the two is a behaviour question for the differ, not a
// reason to call an HTTPRoute missing.
func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if len(p) > 1 {
		return strings.TrimSuffix(p, "/")
	}
	return p
}

func unmatchedFinding(route model.RouteSpec) model.Finding {
	return model.Finding{
		RouteID:   route.ID,
		Severity:  model.SeverityCritical,
		Aspect:    AspectRoute,
		Baseline:  route.Source.String() + " serves " + displayHost(route) + route.Path,
		Candidate: "no HTTPRoute matches this host and path",
		Attribution: []model.Cause{{
			Object: route.Source,
		}},
		Hint: "No Gateway API route carries this traffic. Re-run ingress2gateway for " +
			route.Source.String() + ", or check that the HTTPRoute is in the same namespace " +
			"and that its hostnames and path matches cover this route.",
	}
}

func displayHost(route model.RouteSpec) string {
	if route.Host == "" {
		return "*"
	}
	return route.Host
}
