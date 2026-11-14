package ingressnginx

import (
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Annotation keys.
const (
	AnnPermanentRedirect = Prefix + "permanent-redirect"
	AnnTemporalRedirect  = Prefix + "temporal-redirect"
)

func init() {
	rule.Register(redirect{base{
		id:          "nginx.permanent-redirect",
		annotations: []string{AnnPermanentRedirect},
		hint: "permanent-redirect sends a 301 to a fixed URL. In Gateway API that is a " +
			"RequestRedirect filter with hostname, path and statusCode: 301. Check the " +
			"status code as well as the target: the filter defaults to 302.",
	}})
	rule.Register(redirect{base{
		id:          "nginx.temporal-redirect",
		annotations: []string{AnnTemporalRedirect},
		hint: "temporal-redirect sends a 302 to a fixed URL. Use a RequestRedirect filter " +
			"with statusCode: 302 — the Gateway API default, so usually only the target " +
			"needs writing out.",
	}})
}

// redirect probes a route whose whole job is to send the client elsewhere.
// Both the status code and the target have to survive the migration, and a
// RequestRedirect filter gets the status wrong by default for the permanent
// case.
type redirect struct{ base }

func (d redirect) Generate(r api.RouteSpec) []api.TestCase {
	return []api.TestCase{
		d.get(r, scheme(r), routePath(r),
			api.Assertion{Aspect: api.AspectStatus},
			api.Assertion{Aspect: api.HeaderAspect("Location")},
		),
	}
}
