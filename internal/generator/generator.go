// Package generator turns routes into the concrete requests to probe. It asks
// every rule that applies to the route what it wants tested, and reports the
// annotations no rule covers.
package generator

import (
	"maps"
	"slices"
	"strings"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// NginxPrefix marks the annotations swapsafe is responsible for. Everything
// else on an Ingress — ingress class, Helm and kubectl bookkeeping — is none
// of its business and must not be reported as unverified.
const NginxPrefix = "nginx.ingress.kubernetes.io/"

// Result is what the generator hands to the prober.
type Result struct {
	// TestCases are in route order, then rule order, then the order the rule
	// produced them: stable across runs without needing a sort.
	TestCases []api.TestCase
	// Findings lists ingress-nginx annotations no rule verifies, at info
	// severity. Silence would let an unsupported annotation look like a pass.
	Findings []api.Finding
}

// Generate collects the test cases for every route, dropping duplicate IDs.
func Generate(routes []api.RouteSpec) Result {
	var res Result
	seen := make(map[string]bool)

	for _, route := range routes {
		for _, r := range rule.For(route) {
			for _, tc := range r.Generate(route) {
				// ponytail: deduplicates identical test case IDs only. Two
				// Ingresses serving the same host and path still produce two
				// identical requests, because each belongs to its own route.
				// Deduplicate by request if that ever shows up as real load.
				if seen[tc.ID] {
					continue
				}
				seen[tc.ID] = true
				res.TestCases = append(res.TestCases, tc)
			}
		}
		res.Findings = append(res.Findings, unverifiedAnnotations(route)...)
	}
	return res
}

// unverifiedAnnotations reports every ingress-nginx annotation on the route
// that no registered rule covers, in sorted order.
func unverifiedAnnotations(route api.RouteSpec) []api.Finding {
	var findings []api.Finding

	for _, key := range slices.Sorted(maps.Keys(route.Annotations)) {
		if !strings.HasPrefix(key, NginxPrefix) || rule.Verifies(key) {
			continue
		}
		value := route.Annotations[key]
		findings = append(findings, api.Finding{
			RouteID:   route.ID,
			Severity:  api.SeverityInfo,
			Aspect:    api.AspectAnnotation,
			Baseline:  key + ": " + value,
			Candidate: "not verified",
			Attribution: []api.Cause{{
				Annotation: key,
				Value:      value,
				Object:     route.Source,
			}},
			Hint: "swapsafe has no rule for this annotation yet, so it probes nothing " +
				"specific to it. Check its effect by hand, and see `swapsafe rules` for " +
				"what is covered.",
		})
	}
	return findings
}
