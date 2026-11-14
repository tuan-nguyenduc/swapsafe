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

// Generate collects the test cases for every route. Two rules often want the
// same request — the baseline probes the route path, and so does
// ssl-redirect — so a request is sent once however many rules asked for it.
func Generate(routes []api.RouteSpec) Result {
	var res Result
	seenID := make(map[string]bool)

	// Request signature to its index in res.TestCases. Scoped per route: the
	// same request on two routes is still two findings to attribute.
	for _, route := range routes {
		seen := make(map[string]int)

		for _, r := range rule.For(route) {
			for _, tc := range r.Generate(route) {
				if seenID[tc.ID] {
					continue
				}
				sig := signature(tc.Request)
				if i, dup := seen[sig]; dup {
					// A rule that knows why it is asking beats the generic
					// probe, so its hint is the one the user reads.
					if res.TestCases[i].RuleID == rule.BaselineID && tc.RuleID != rule.BaselineID {
						res.TestCases[i] = tc
					}
					continue
				}
				seenID[tc.ID] = true
				seen[sig] = len(res.TestCases)
				res.TestCases = append(res.TestCases, tc)
			}
			if reporter, ok := r.(rule.Reporter); ok {
				res.Findings = append(res.Findings, reporter.Report(route)...)
			}
		}
		res.Findings = append(res.Findings, unverifiedAnnotations(route)...)
	}
	return res
}

// signature identifies a request, so the same one is never sent twice.
//
// ponytail: scoped to one route. Two Ingresses serving the same host and path
// still produce two identical requests, because each belongs to its own
// route and its own findings. Widen it if that ever shows up as real load.
func signature(r api.RequestSpec) string {
	var sb strings.Builder
	sb.WriteString(r.Method + "\x00" + r.Scheme + "\x00" + r.Path + "\x00" + r.Query)
	for _, name := range slices.Sorted(maps.Keys(r.Headers)) {
		sb.WriteString("\x00" + name + ":" + strings.Join(r.Headers[name], ","))
	}
	return sb.String()
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
