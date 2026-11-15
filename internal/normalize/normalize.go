// Package normalize strips the parts of a response that differ on every
// request, so the differ only sees differences that mean something. A raw
// diff of two proxies is almost entirely noise (PLAN section 5.4).
package normalize

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// ponytail: headers only. PLAN 5.4 also canonicalizes a JSON body before
// comparing its hash, which has to happen in the prober, where the bytes
// still exist. Skipped because key order is the backend's doing, not the
// proxy's: both endpoints talk to the same backend. Add it if a real run
// produces body findings that turn out to be key order.

// Redacted replaces any value swapsafe must never write to a report.
const Redacted = "REDACTED"

// dropHeaders are dropped before comparing: they differ on every request, or
// on every proxy, without the behaviour differing at all. Content-Length goes
// too, because the body is compared by hash.
//
// The second group is the hop-by-hop set from RFC 9110 section 7.6.1. These
// describe the connection between client and proxy, not the answer the
// application gave, and two proxies routinely disagree about them: the
// end-to-end suite caught ingress-nginx sending Connection: keep-alive where
// Envoy sends nothing at all.
var dropHeaders = []string{
	"Content-Length",
	"Date",
	"Server",
	"Traceparent",
	"Via",
	"X-Request-Id",

	"Connection",
	"Keep-Alive",
	"Proxy-Connection",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// dropPrefixes drops whole families of implementation headers.
var dropPrefixes = []string{"X-Envoy-"}

// maskHeaders are kept — their presence is behaviour — but never their value
// (PLAN section 6.3).
var maskHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Www-Authenticate",
	"Proxy-Authenticate",
}

// Options carries the user's additions from swapsafe.yaml.
type Options struct {
	// IgnoreHeaders are dropped on top of the built-in list.
	IgnoreHeaders []string
}

// Observation returns obs with its headers normalized. The request is needed
// to resolve a relative Location.
func Observation(obs api.Observation, req api.RequestSpec, opts Options) api.Observation {
	obs.Headers = Headers(obs.Headers, req, opts)
	return obs
}

// Headers drops the noisy headers, masks the secret ones, rewrites Location
// and Set-Cookie into a comparable form, and sorts multi-value headers so
// that ordering alone is never a difference.
func Headers(h http.Header, req api.RequestSpec, opts Options) http.Header {
	if len(h) == 0 {
		return nil
	}

	out := make(http.Header, len(h))
	for name, values := range h {
		name = http.CanonicalHeaderKey(name)
		if dropped(name, opts) {
			continue
		}

		normalized := make([]string, 0, len(values))
		for _, v := range values {
			switch {
			case slices.Contains(maskHeaders, name):
				v = Redacted
			case name == "Location":
				v = Location(v, req)
			case name == "Set-Cookie":
				v = SetCookie(v)
			}
			normalized = append(normalized, v)
		}
		slices.Sort(normalized)
		out[name] = slices.Compact(normalized)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func dropped(name string, opts Options) bool {
	if slices.Contains(dropHeaders, name) {
		return true
	}
	for _, p := range dropPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, extra := range opts.IgnoreHeaders {
		if http.CanonicalHeaderKey(extra) == name {
			return true
		}
	}
	return false
}

// Location rewrites a redirect target into an absolute, canonical URL:
// relative targets are resolved against the request, the scheme and host are
// lowercased, a default port is dropped, and percent-encoding is normalized.
// Two proxies writing the same destination differently must compare equal.
func Location(raw string, req api.RequestSpec) string {
	if raw == "" {
		return ""
	}

	ref, err := url.Parse(raw)
	if err != nil {
		// Not a URL at all. Leave it alone: the differ reporting two
		// unparsable values as different is the honest outcome.
		return raw
	}

	base := &url.URL{
		Scheme: strings.ToLower(req.Scheme),
		Host:   strings.ToLower(req.Host),
		Path:   req.Path,
	}
	abs := base.ResolveReference(ref)
	abs.Scheme = strings.ToLower(abs.Scheme)
	abs.Host = stripDefaultPort(strings.ToLower(abs.Host), abs.Scheme)
	return abs.String()
}

// stripDefaultPort removes :80 from an http URL and :443 from an https one.
func stripDefaultPort(host, scheme string) string {
	h, port, found := strings.Cut(host, ":")
	if !found {
		return host
	}
	// An IPv6 literal keeps its brackets and may hold colons of its own.
	if strings.HasPrefix(host, "[") {
		idx := strings.LastIndex(host, "]:")
		if idx < 0 {
			return host
		}
		h, port = host[:idx+1], host[idx+2:]
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return h
	}
	return host
}

// SetCookie reduces a cookie to what is worth comparing: the name and the
// attributes that change client behaviour. The value is always redacted, and
// Expires and Max-Age are dropped because they move with the clock.
func SetCookie(raw string) string {
	c, err := http.ParseSetCookie(raw)
	if err != nil {
		// Never echo back something unparsable: it could be anything.
		return "unparsable set-cookie"
	}

	parts := []string{c.Name + "=" + Redacted}
	if c.Path != "" {
		parts = append(parts, "Path="+c.Path)
	}
	if c.Domain != "" {
		parts = append(parts, "Domain="+strings.ToLower(c.Domain))
	}
	if c.Secure {
		parts = append(parts, "Secure")
	}
	if c.HttpOnly {
		parts = append(parts, "HttpOnly")
	}
	// ParseSetCookie leaves SameSite at zero when the attribute is absent,
	// which is a different thing from an explicit SameSite=Default.
	if s := sameSite(c.SameSite); s != "" {
		parts = append(parts, "SameSite="+s)
	}
	if c.Partitioned {
		parts = append(parts, "Partitioned")
	}
	return strings.Join(parts, "; ")
}

func sameSite(s http.SameSite) string {
	switch s {
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteNoneMode:
		return "None"
	default:
		return ""
	}
}
