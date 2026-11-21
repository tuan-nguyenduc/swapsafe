package collector

import (
	"context"
	"sort"

	networkingv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// Result is what the collector hands to the rest of the pipeline.
type Result struct {
	// Routes are sorted by ID, so every stage downstream is deterministic.
	Routes []api.RouteSpec
	// Findings reports routes with no matching HTTPRoute. Those are critical:
	// the traffic has nowhere to go after the cutover.
	Findings []api.Finding
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
// same namespace, a hostname covering the route's host, and a path match that
// covers the route's path.
//
// Where several HTTPRoutes cover it, the most specific one wins — the longest
// prefix — which is how Gateway API itself picks.
func findTarget(route api.RouteSpec, httpRoutes []gatewayv1.HTTPRoute) (api.ObjectRef, bool) {
	want := api.LiteralPathPrefix(route.Path)

	best := -1
	var ref api.ObjectRef

	for _, hr := range httpRoutes {
		if hr.Namespace != route.Source.Namespace {
			continue
		}
		if !hostnamesCover(hr.Spec.Hostnames, route.Host) {
			continue
		}

		specificity := rulesCoverage(hr.Spec.Rules, want)
		if specificity < 0 {
			continue
		}
		candidate := api.ObjectRef{
			Group:     gatewayv1.GroupName,
			Kind:      "HTTPRoute",
			Namespace: hr.Namespace,
			Name:      hr.Name,
		}
		// A tie is broken by name, so two equally specific HTTPRoutes always
		// pair the same way across runs.
		if specificity > best || (specificity == best && candidate.Name < ref.Name) {
			best, ref = specificity, candidate
		}
	}
	return ref, best >= 0
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

// rulesCoverage returns the length of the longest path match that covers
// want, or -1 when no rule covers it. The length is the specificity: between
// an HTTPRoute matching /podinfo and one matching /, the first is the one the
// traffic actually reaches.
func rulesCoverage(rules []gatewayv1.HTTPRouteRule, want string) int {
	best := -1
	for _, rule := range rules {
		if len(rule.Matches) == 0 {
			// Defaults to PathPrefix "/", which covers everything.
			best = max(best, 1)
			continue
		}
		for _, m := range rule.Matches {
			best = max(best, pathCoverage(m.Path, want))
		}
	}
	return best
}

// pathCoverage scores one HTTPRoute path match against the Ingress path, or
// returns -1 when it does not cover it.
//
// The comparison is of configuration, not of behaviour: proving the two agree
// is what the probing pipeline does. Here we only need to decide which
// HTTPRoute is the counterpart of which Ingress route.
func pathCoverage(p *gatewayv1.HTTPPathMatch, want string) int {
	if p == nil || p.Value == nil {
		return 1 // PathPrefix "/" by default
	}
	value := api.NormalizePath(*p.Value)

	matchType := gatewayv1.PathMatchPathPrefix
	if p.Type != nil {
		matchType = *p.Type
	}

	switch matchType {
	case gatewayv1.PathMatchPathPrefix:
		if api.PathPrefixCovers(value, want) {
			return len(value)
		}
	case gatewayv1.PathMatchExact:
		if value == want {
			return len(value)
		}
	case gatewayv1.PathMatchRegularExpression:
		// ponytail: two regular expressions are only paired when they are
		// written identically. Deciding whether one regex covers another needs
		// a solver, and the probes are the cheaper answer.
		if value == want {
			return len(value)
		}
	}
	return -1
}

func unmatchedFinding(route api.RouteSpec) api.Finding {
	return api.Finding{
		RouteID:  route.ID,
		Severity: api.SeverityCritical,
		Aspect:   api.AspectRoute,
		// Terse on purpose: the table has five other columns, and the hint
		// below carries the whole story.
		Baseline:  "Ingress " + route.Source.Namespace + "/" + route.Source.Name,
		Candidate: "no HTTPRoute",
		Attribution: []api.Cause{{
			Object: route.Source,
		}},
		Hint: "Nothing on the Gateway side carries the traffic for " + displayHost(route) +
			route.Path + ". Re-run ingress2gateway for " + route.Source.String() +
			", or check that the HTTPRoute is in the same namespace and that its " +
			"hostnames and path matches cover this route.",
	}
}

func displayHost(route api.RouteSpec) string {
	if route.Host == "" {
		return "*"
	}
	return route.Host
}
