// Package ingressnginx holds one Rule per ingress-nginx annotation. Importing
// it registers them all.
//
// These rules may only import pkg/rule and pkg/api, so the whole package can
// be lifted into its own module later (PLAN section 4, ADR-0002).
package ingressnginx

import (
	"net/http"
	"strings"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Prefix is what every ingress-nginx annotation starts with.
const Prefix = "nginx.ingress.kubernetes.io/"

// probeSegment is appended when a rule needs a path that exists in form but
// not in fact, to see how each side handles it.
const probeSegment = "swapsafe-probe"

// base carries what every rule here shares, so each file only writes the part
// that is actually specific to its annotation.
type base struct {
	id          string
	annotations []string
	hint        string
}

func (b base) ID() string            { return b.id }
func (b base) Annotations() []string { return b.annotations }

// Applies: the rule is active when the route carries any of its annotations.
func (b base) Applies(r api.RouteSpec) bool {
	for _, name := range b.annotations {
		if _, ok := r.Annotations[name]; ok {
			return true
		}
	}
	return false
}

// Compare adds nothing by default: the differ already compares every aspect
// of the two responses. A rule overrides this only for a judgement the
// generic comparison cannot make.
func (base) Compare(api.TestCase, api.Observation, api.Observation) []api.Finding { return nil }

func (b base) Hint(api.Finding) string { return b.hint }

// get builds a GET test case for a rule, over a given scheme and path.
func (b base) get(r api.RouteSpec, scheme, path string, expect ...api.Assertion) api.TestCase {
	return b.request(r, http.MethodGet, scheme, path, nil, expect...)
}

func (b base) request(r api.RouteSpec, method, scheme, path string, headers http.Header, expect ...api.Assertion) api.TestCase {
	variant := method + " " + scheme + " " + path
	for _, name := range sortedNames(headers) {
		variant += " " + name + ":" + strings.Join(headers[name], ",")
	}

	return api.TestCase{
		ID:      rule.TestCaseID(r.ID, b.id, variant),
		RouteID: r.ID,
		RuleID:  b.id,
		Request: api.RequestSpec{
			Method:  method,
			Scheme:  scheme,
			Host:    r.Host,
			Path:    path,
			Headers: headers,
		},
		Expect: expect,
	}
}

func sortedNames(h http.Header) []string {
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	// Few headers ever, and they must land in a stable order for the hash.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// value returns an annotation's value.
func value(r api.RouteSpec, name string) string { return r.Annotations[name] }

// scheme is the scheme a route is normally reached over.
func scheme(r api.RouteSpec) string {
	if r.TLS {
		return "https"
	}
	return "http"
}

// routePath is the route's path, defaulting to the site root.
func routePath(r api.RouteSpec) string {
	if r.Path == "" {
		return "/"
	}
	return r.Path
}

// subPath is a path underneath the route, used to see how each side handles
// something the backend will not recognize.
func subPath(r api.RouteSpec) string {
	return strings.TrimSuffix(routePath(r), "/") + "/" + probeSegment
}
