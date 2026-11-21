package ingressnginx

import (
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Annotation keys.
const (
	AnnRewriteTarget = Prefix + "rewrite-target"
	AnnUseRegex      = Prefix + "use-regex"
)

func init() {
	rule.Register(rewriteTarget{base{
		id:          "nginx.rewrite-target",
		annotations: []string{AnnRewriteTarget},
		hint: "rewrite-target changes the path the backend sees. Gateway API expresses the " +
			"common case — strip the matched prefix — with a URLRewrite filter using " +
			"path.type: ReplacePrefixMatch. A rewrite-target with capture groups ($1, $2) " +
			"has no Gateway API equivalent and needs the route splitting up by hand.",
	}})
	rule.Register(useRegex{base{
		id:          "nginx.use-regex",
		annotations: []string{AnnUseRegex},
		hint: "use-regex makes the Ingress path a regular expression. Core Gateway API " +
			"matches only Exact and PathPrefix; RegularExpression is optional and " +
			"implementation-specific, so check your Gateway implementation supports it " +
			"before trusting the translation.",
	}})
}

// rewriteTarget probes a path under the route. The annotation only shows
// itself in what the backend receives, so the signal is the response body
// coming back different, or a 404 appearing on one side.
type rewriteTarget struct{ base }

func (rw rewriteTarget) Generate(r api.RouteSpec) []api.TestCase {
	if value(r, AnnRewriteTarget) == "" {
		return nil
	}
	expect := []api.Assertion{
		{Aspect: api.AspectStatus},
		{Aspect: api.AspectBody},
	}
	return []api.TestCase{
		// The route path itself, which a prefix-stripping rewrite turns into
		// the backend root — a page that exists, so a wrong rewrite shows up
		// as a different body instead of as two identical 404s.
		rw.get(r, scheme(r), routePath(r), expect...),
		// And one level below it, where the captured part of the path is
		// what the backend receives.
		rw.get(r, scheme(r), subPath(r), expect...),
	}
}

// useRegex probes a concrete path the regex should match, since the literal
// Ingress path is a pattern and not a URL anyone can request.
type useRegex struct{ base }

func (u useRegex) Generate(r api.RouteSpec) []api.TestCase {
	if value(r, AnnUseRegex) != "true" {
		return nil
	}
	return []api.TestCase{
		u.get(r, scheme(r), subPath(r),
			api.Assertion{Aspect: api.AspectStatus},
			api.Assertion{Aspect: api.AspectBody},
		),
	}
}
