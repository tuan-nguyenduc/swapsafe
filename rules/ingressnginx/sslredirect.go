package ingressnginx

import (
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Annotation keys.
const (
	AnnSSLRedirect      = Prefix + "ssl-redirect"
	AnnForceSSLRedirect = Prefix + "force-ssl-redirect"
)

func init() {
	rule.Register(sslRedirect{base{
		id:          "nginx.ssl-redirect",
		annotations: []string{AnnSSLRedirect},
		hint: "ingress-nginx redirects plain HTTP to HTTPS itself. Gateway API has no " +
			"equivalent annotation: add a second listener on port 80 with an HTTPRoute " +
			"carrying a RequestRedirect filter (scheme: https, statusCode: 301). " +
			"ingress-nginx answers 308 by default, so expect a status difference unless " +
			"you match it.",
	}})
	rule.Register(sslRedirect{base{
		id:          "nginx.force-ssl-redirect",
		annotations: []string{AnnForceSSLRedirect},
		hint: "force-ssl-redirect redirects HTTP to HTTPS even for routes with no TLS " +
			"block. Reproduce it with a port 80 listener and an HTTPRoute whose only " +
			"filter is a RequestRedirect to scheme https.",
	}})
}

// sslRedirect probes what each side does with a plain HTTP request. That is
// the whole behaviour of both annotations, and the single most common thing
// to lose in a migration: the Gateway serves the page instead of redirecting.
type sslRedirect struct{ base }

func (s sslRedirect) Generate(r api.RouteSpec) []api.TestCase {
	// Only "false" turns it off; ingress-nginx treats any other value as on.
	if value(r, s.annotations[0]) == "false" {
		return nil
	}
	return []api.TestCase{
		s.get(r, "http", routePath(r),
			api.Assertion{Aspect: api.AspectStatus},
			api.Assertion{Aspect: api.HeaderAspect("Location")},
		),
	}
}
