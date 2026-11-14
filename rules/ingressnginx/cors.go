package ingressnginx

import (
	"net/http"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Annotation keys. enable-cors turns it on; the rest tune it.
const (
	AnnEnableCORS           = Prefix + "enable-cors"
	AnnCORSAllowOrigin      = Prefix + "cors-allow-origin"
	AnnCORSAllowMethods     = Prefix + "cors-allow-methods"
	AnnCORSAllowHeaders     = Prefix + "cors-allow-headers"
	AnnCORSAllowCredentials = Prefix + "cors-allow-credentials"
	AnnCORSExposeHeaders    = Prefix + "cors-expose-headers"
	AnnCORSMaxAge           = Prefix + "cors-max-age"
)

// probeOrigin is the Origin swapsafe presents. It is deliberately a domain
// nobody allow-lists, so an endpoint that echoes it back is being permissive
// and an endpoint that does not is being correct — and either way both sides
// have to agree.
const probeOrigin = "https://swapsafe.invalid"

func init() {
	rule.Register(cors{base{
		id: "nginx.enable-cors",
		annotations: []string{
			AnnEnableCORS, AnnCORSAllowOrigin, AnnCORSAllowMethods, AnnCORSAllowHeaders,
			AnnCORSAllowCredentials, AnnCORSExposeHeaders, AnnCORSMaxAge,
		},
		hint: "ingress-nginx answers CORS preflights itself. Gateway API has a CORS filter " +
			"in v1.3 and later, but implementations vary: check your Gateway supports it, " +
			"and that allow-origin, allow-methods, allow-headers and allow-credentials " +
			"were all carried over, not just the first one.",
	}})
}

// cors sends a preflight and a simple cross-origin request. CORS is invisible
// to an ordinary probe: nothing changes unless the request carries an Origin.
type cors struct{ base }

func (c cors) Generate(r api.RouteSpec) []api.TestCase {
	if value(r, AnnEnableCORS) == "false" {
		return nil
	}

	expect := []api.Assertion{
		{Aspect: api.AspectStatus},
		// PLAN 5.4: a missing CORS header is major even when the status matches,
		// because the browser, not the proxy, is what breaks.
		{Aspect: api.HeaderAspect("Access-Control-Allow-Origin"), Severity: api.SeverityMajor},
		{Aspect: api.HeaderAspect("Access-Control-Allow-Methods"), Severity: api.SeverityMajor},
		{Aspect: api.HeaderAspect("Access-Control-Allow-Headers"), Severity: api.SeverityMajor},
		{Aspect: api.HeaderAspect("Access-Control-Allow-Credentials"), Severity: api.SeverityMajor},
	}

	preflight := http.Header{
		"Origin":                         {probeOrigin},
		"Access-Control-Request-Method":  {http.MethodGet},
		"Access-Control-Request-Headers": {"content-type"},
	}

	return []api.TestCase{
		c.request(r, http.MethodOptions, scheme(r), routePath(r), preflight, expect...),
		c.request(r, http.MethodGet, scheme(r), routePath(r), http.Header{"Origin": {probeOrigin}}, expect...),
	}
}
