package ingressnginx

import (
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// AnnAppRoot redirects requests for "/" to a sub-path.
const AnnAppRoot = Prefix + "app-root"

func init() {
	rule.Register(appRoot{base{
		id:          "nginx.app-root",
		annotations: []string{AnnAppRoot},
		hint: "app-root redirects a request for / to the configured path. Gateway API has " +
			"no annotation for it: add an HTTPRoute rule matching Exact / with a " +
			"RequestRedirect filter whose path is a ReplaceFullPath to that value. " +
			"ingress-nginx answers 302 here.",
	}})
}

// appRoot probes the site root rather than the route path: the annotation
// only ever affects "/", and a route mounted deeper would never see it.
type appRoot struct{ base }

func (a appRoot) Generate(r api.RouteSpec) []api.TestCase {
	if value(r, AnnAppRoot) == "" {
		return nil
	}
	return []api.TestCase{
		a.get(r, scheme(r), "/",
			api.Assertion{Aspect: api.AspectStatus},
			api.Assertion{Aspect: api.HeaderAspect("Location")},
		),
	}
}
