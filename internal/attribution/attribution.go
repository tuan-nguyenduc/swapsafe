// Package attribution answers the question a diff alone cannot: which
// annotation caused this difference, and what should the user change. It runs
// after the differ, because it needs the routes and the rule registry, and
// the differ needs neither.
package attribution

import (
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Apply fills in Attribution and Hint on every finding that does not already
// have them, and returns the findings. Findings from the collector and the
// generator arrive already attributed and are left alone.
func Apply(findings []api.Finding, routes []api.RouteSpec, cases []api.TestCase) []api.Finding {
	routesByID := index(routes, func(r api.RouteSpec) string { return r.ID })
	casesByID := index(cases, func(tc api.TestCase) string { return tc.ID })
	rulesByID := index(rule.All(), rule.Rule.ID)

	out := make([]api.Finding, 0, len(findings))
	for _, f := range findings {
		route, haveRoute := routesByID[f.RouteID]
		tc, haveCase := casesByID[f.TestCaseID]

		if len(f.Attribution) == 0 && haveRoute {
			var r rule.Rule
			if haveCase {
				r = rulesByID[tc.RuleID]
			}
			f.Attribution = causes(route, r)
		}
		if f.Hint == "" && haveCase {
			if r, ok := rulesByID[tc.RuleID]; ok {
				f.Hint = r.Hint(f)
			}
		}
		out = append(out, f)
	}
	return out
}

// causes lists the annotations on the route that the rule behind this finding
// reads. A rule driven by no annotation — the baseline — still points at the
// source Ingress, so every finding names an object the user can go and look at.
func causes(route api.RouteSpec, r rule.Rule) []api.Cause {
	var out []api.Cause
	if r != nil {
		for _, key := range r.Annotations() {
			value, ok := route.Annotations[key]
			if !ok {
				continue
			}
			out = append(out, api.Cause{Annotation: key, Value: value, Object: route.Source})
		}
	}
	if len(out) == 0 {
		out = []api.Cause{{Object: route.Source}}
	}
	return out
}

func index[T any](items []T, key func(T) string) map[string]T {
	out := make(map[string]T, len(items))
	for _, item := range items {
		out[key(item)] = item
	}
	return out
}
