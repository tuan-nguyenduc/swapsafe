package rule

import (
	"net/http"
	"strings"

	"github.com/tuan-nguyenduc/swapsafe/internal/model"
)

// BaselineID is the rule that runs for every route, annotations or not.
const BaselineID = "baseline"

// probeSegment is appended to a prefix path to see how each side handles a
// sub-path. It is deliberately unlikely to exist: a 404 from both endpoints
// is a match, and a 404 from only one is exactly the kind of rewrite bug
// swapsafe is looking for.
const probeSegment = "swapsafe-probe"

func init() { Register(baselineRule{}) }

// baselineRule probes the route itself: the path as written, the same path
// with the trailing slash flipped, and a sub-path (PLAN section 5.2).
type baselineRule struct{}

func (baselineRule) ID() string { return BaselineID }

// Annotations is empty: this rule is not driven by one.
func (baselineRule) Annotations() []string { return nil }

func (baselineRule) Applies(model.RouteSpec) bool { return true }

func (baselineRule) Generate(r model.RouteSpec) []model.TestCase {
	var cases []model.TestCase
	for _, scheme := range baselineSchemes(r) {
		for _, path := range baselinePaths(r) {
			variant := scheme + " " + path
			cases = append(cases, model.TestCase{
				ID:      TestCaseID(r.ID, BaselineID, variant),
				RouteID: r.ID,
				RuleID:  BaselineID,
				Request: model.RequestSpec{
					Method: http.MethodGet,
					Scheme: scheme,
					Host:   r.Host,
					Path:   path,
				},
				Expect: []model.Assertion{
					{Aspect: "status"},
					{Aspect: "header:location"},
					{Aspect: "body"},
				},
			})
		}
	}
	return cases
}

// Compare adds nothing: the differ already compares the aspects listed in
// Expect. A rule only implements Compare when it needs a judgement the
// generic comparison cannot make.
func (baselineRule) Compare(model.TestCase, model.Observation, model.Observation) []model.Finding {
	return nil
}

func (baselineRule) Hint(f model.Finding) string {
	switch {
	case f.Aspect == "status":
		return "The two endpoints answer this request with different status codes. " +
			"Check the HTTPRoute path match type: an Ingress Prefix is PathPrefix, " +
			"an Ingress Exact is Exact, and the two differ on sub-paths."
	case f.Aspect == "header:location":
		return "Only one side redirects, or they redirect elsewhere. Look for a " +
			"RequestRedirect or URLRewrite filter on the HTTPRoute rule."
	case strings.HasPrefix(f.Aspect, "body"):
		return "Same status, different body: the request is most likely reaching a " +
			"different backend. Compare the HTTPRoute backendRefs with the Ingress backend."
	default:
		return ""
	}
}

// baselineSchemes probes https only for routes the Ingress terminates TLS for.
// Probing https on a plaintext route would fail on both sides and report a
// difference that is really just an absent listener.
func baselineSchemes(r model.RouteSpec) []string {
	if r.TLS {
		return []string{"http", "https"}
	}
	return []string{"http"}
}

// baselinePaths returns the path variants to probe, in a stable order and
// with no duplicates.
func baselinePaths(r model.RouteSpec) []string {
	path := r.Path
	if path == "" {
		path = "/"
	}

	paths := []string{path}
	if alt := toggleTrailingSlash(path); alt != "" {
		paths = append(paths, alt)
	}
	// An Exact path has no sub-paths by definition.
	if r.PathType != model.PathTypeExact {
		paths = append(paths, subPath(path))
	}
	return paths
}

// toggleTrailingSlash returns the same path with the trailing slash flipped,
// or "" for "/", where there is nothing to flip.
func toggleTrailingSlash(path string) string {
	switch {
	case path == "/":
		return ""
	case strings.HasSuffix(path, "/"):
		return strings.TrimSuffix(path, "/")
	default:
		return path + "/"
	}
}

func subPath(path string) string {
	return strings.TrimSuffix(path, "/") + "/" + probeSegment
}
