// Package differ compares the two normalized responses to a test case and
// turns every meaningful difference into a Finding. It is where swapsafe
// earns its keep, so it is deliberately strict about what counts as
// meaningful: normalization has already removed the noise.
//
// It takes observations that are already normalized, and it does not know
// which annotation caused what — that is the attribution stage's job.
package differ

import (
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// DiffAll compares every test case that has a pair of observations, in test
// case order.
func DiffAll(cases []api.TestCase, observations map[string][]api.Observation) []api.Finding {
	var out []api.Finding
	for _, tc := range cases {
		base, cand, ok := pair(observations[tc.ID])
		if !ok {
			continue
		}
		out = append(out, Diff(tc, base, cand)...)
	}
	return out
}

// pair picks the baseline and candidate observations out of a test case's
// results. A case with only one of them was never fully probed.
func pair(obs []api.Observation) (base, cand api.Observation, ok bool) {
	var haveBase, haveCand bool
	for _, o := range obs {
		switch o.Endpoint {
		case api.EndpointBaseline:
			base, haveBase = o, true
		case api.EndpointCandidate:
			cand, haveCand = o, true
		}
	}
	return base, cand, haveBase && haveCand
}

// Diff compares one pair of normalized observations.
func Diff(tc api.TestCase, base, cand api.Observation) []api.Finding {
	expected := expectedSeverities(tc)

	var out []api.Finding
	add := func(aspect string, sev api.Severity, baseValue, candValue string) {
		if override, ok := expected[aspect]; ok && override != "" {
			sev = override
		}
		out = append(out, api.Finding{
			RouteID:    tc.RouteID,
			TestCaseID: tc.ID,
			Severity:   sev,
			Aspect:     aspect,
			Baseline:   baseValue,
			Candidate:  candValue,
		})
	}

	switch {
	case base.Err != "" && cand.Err != "":
		// Both ends failed. That is not a difference between them; it is a
		// problem with the run, and the report says so by having no finding
		// here and an error on both observations.
		return nil
	case base.Err != "" || cand.Err != "":
		add(api.AspectError, api.SeverityCritical, errorOrOK(base), errorOrOK(cand))
		// Nothing else about the two is comparable.
		return out
	}

	if base.Status != cand.Status {
		add(api.AspectStatus, statusSeverity(base.Status, cand.Status),
			strconv.Itoa(base.Status), strconv.Itoa(cand.Status))
	}

	// net/http canonicalizes header names on the wire, but an observation
	// read back from results.json need not be, and capitalization is never a
	// behaviour difference.
	baseHeaders, candHeaders := canonical(base.Headers), canonical(cand.Headers)
	for _, name := range headerNames(baseHeaders, candHeaders) {
		baseValue, candValue := headerValue(baseHeaders, name), headerValue(candHeaders, name)
		if baseValue == candValue {
			continue
		}
		add(api.HeaderAspect(name), headerSeverity(name, baseValue, candValue), baseValue, candValue)
	}

	// The body is compared by hash, so a difference says "not the same bytes"
	// and nothing more. With the status equal, that usually means the request
	// reached a different backend — unless the baseline does not return the
	// same bytes twice itself, in which case there is nothing to compare and
	// saying otherwise would be a lie.
	switch {
	case base.BodyUnstable || cand.BodyUnstable:
		add(api.AspectBody, api.SeverityInfo,
			"changes between identical requests", "not compared")
	case base.BodySHA256 != cand.BodySHA256:
		add(api.AspectBody, api.SeverityMajor, shortHash(base.BodySHA256), shortHash(cand.BodySHA256))
	}

	return out
}

// expectedSeverities collects the severities the rule assigned per aspect.
func expectedSeverities(tc api.TestCase) map[string]api.Severity {
	if len(tc.Expect) == 0 {
		return nil
	}
	out := make(map[string]api.Severity, len(tc.Expect))
	for _, a := range tc.Expect {
		out[a.Aspect] = a.Severity
	}
	return out
}

// statusSeverity follows PLAN section 5.4: crossing a status class is
// critical, because a 2xx and a 4xx are not the same answer by any reading.
// Within a class — 301 against 308, 200 against 204 — the behaviour is close
// enough to be a judgement call, so it is minor.
func statusSeverity(base, cand int) api.Severity {
	if base/100 != cand/100 {
		return api.SeverityCritical
	}
	return api.SeverityMinor
}

// headerSeverity: a redirect target is always major, since it decides where
// the user ends up. For anything else, a header present on one side only
// changes behaviour, while a different value is usually cosmetic.
func headerSeverity(name, baseValue, candValue string) api.Severity {
	if http.CanonicalHeaderKey(name) == "Location" {
		return api.SeverityMajor
	}
	if baseValue == api.ValueAbsent || candValue == api.ValueAbsent {
		return api.SeverityMajor
	}
	return api.SeverityMinor
}

// canonical returns h with every name in canonical form.
func canonical(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for name, values := range h {
		out[http.CanonicalHeaderKey(name)] = values
	}
	return out
}

// headerNames returns every header either side sent, sorted, so the findings
// come out in the same order on every run.
func headerNames(a, b http.Header) []string {
	names := make(map[string]bool, len(a)+len(b))
	for name := range a {
		names[http.CanonicalHeaderKey(name)] = true
	}
	for name := range b {
		names[http.CanonicalHeaderKey(name)] = true
	}
	return slices.Sorted(maps.Keys(names))
}

// headerValue joins a multi-value header. normalize has already sorted the
// values, so this is stable.
func headerValue(h http.Header, name string) string {
	values, ok := h[http.CanonicalHeaderKey(name)]
	if !ok || len(values) == 0 {
		return api.ValueAbsent
	}
	return strings.Join(values, ", ")
}

func errorOrOK(o api.Observation) string {
	if o.Err == "" {
		return "answered " + strconv.Itoa(o.Status)
	}
	return o.Err
}

// shortHash keeps a report readable: the full digest says nothing a human
// can use, and the first bytes are enough to tell two bodies apart.
func shortHash(sum string) string {
	if sum == "" {
		return api.ValueAbsent
	}
	if len(sum) > 12 {
		return "sha256:" + sum[:12]
	}
	return "sha256:" + sum
}

// ConfirmRouteFindings re-reads the "no HTTPRoute" findings in the light of
// what the probes actually saw.
//
// Pairing is static, and static answers can be wrong: a broader PathPrefix,
// or an idiom the collector does not recognize, serves the route even though
// nothing was paired with it. When both endpoints answer that route the same
// way, the honest verdict is not critical — it is that something is serving
// it and the user should find out what.
func ConfirmRouteFindings(findings []api.Finding, cases []api.TestCase, observations map[string][]api.Observation) []api.Finding {
	byRoute := make(map[string][]string, len(cases))
	for _, tc := range cases {
		byRoute[tc.RouteID] = append(byRoute[tc.RouteID], tc.ID)
	}

	out := make([]api.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Aspect == api.AspectRoute && served(byRoute[f.RouteID], observations) {
			f.Severity = api.SeverityInfo
			f.Candidate = "served anyway, by some other route"
			f.Hint = "No HTTPRoute could be paired with this route, but both endpoints " +
				"answer it the same way, so something on the Gateway side is carrying " +
				"the traffic — most likely a broader PathPrefix. Find out which " +
				"HTTPRoute matches and confirm that is what you meant."
		}
		out = append(out, f)
	}
	return out
}

// served reports whether every probe of a route came back the same from both
// endpoints. No probes at all means nothing was confirmed.
func served(caseIDs []string, observations map[string][]api.Observation) bool {
	var probed bool
	for _, id := range caseIDs {
		base, cand, ok := pair(observations[id])
		if !ok {
			continue
		}
		probed = true
		if base.Err != "" || cand.Err != "" || base.Status != cand.Status {
			return false
		}
		if !base.BodyUnstable && !cand.BodyUnstable && base.BodySHA256 != cand.BodySHA256 {
			return false
		}
	}
	return probed
}
