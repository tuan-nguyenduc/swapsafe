package ingressnginx

import (
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
	"github.com/tuan-nguyenduc/swapsafe/pkg/rule"
)

// Annotation keys for behaviour swapsafe cannot observe from outside.
const (
	AnnProxyBodySize   = Prefix + "proxy-body-size"
	AnnBackendProtocol = Prefix + "backend-protocol"
)

func init() {
	rule.Register(reportOnly{
		base: base{
			id:          "nginx.proxy-body-size",
			annotations: []string{AnnProxyBodySize},
			hint: "proxy-body-size caps the request body. Gateway API has no equivalent, " +
				"and the limit then falls to your Gateway implementation's own setting — " +
				"Envoy Gateway's ClientTrafficPolicy, for instance. Set it there and " +
				"confirm with a request larger than the limit.",
		},
		severity: api.SeverityMajor,
		why: "swapsafe only sends GET, HEAD and OPTIONS, so it cannot show you what " +
			"either side does with an oversized body",
	})
	rule.Register(reportOnly{
		base: base{
			id:          "nginx.backend-protocol",
			annotations: []string{AnnBackendProtocol},
			hint: "backend-protocol changes how the proxy talks to the backend. GRPC needs " +
				"a GRPCRoute rather than an HTTPRoute; HTTPS or GRPCS needs a " +
				"BackendTLSPolicy. ingress2gateway does not translate this, so check it " +
				"by hand.",
		},
		severity: api.SeverityMajor,
		why: "the protocol between proxy and backend is not visible from a client, so " +
			"probing cannot confirm it",
	})
}

// reportOnly is a rule that has something to say but nothing to probe.
type reportOnly struct {
	base
	severity api.Severity
	why      string
}

// Generate sends nothing: see why.
func (reportOnly) Generate(api.RouteSpec) []api.TestCase { return nil }

func (ro reportOnly) Report(route api.RouteSpec) []api.Finding {
	var out []api.Finding
	for _, name := range ro.annotations {
		v, ok := route.Annotations[name]
		if !ok {
			continue
		}
		out = append(out, api.Finding{
			RouteID:   route.ID,
			Severity:  ro.severity,
			Aspect:    api.AspectAnnotation,
			Baseline:  name + ": " + v,
			Candidate: "not verifiable by probing",
			Attribution: []api.Cause{{
				Annotation: name,
				Value:      v,
				Object:     route.Source,
			}},
			Hint: ro.why + ". " + ro.hint,
		})
	}
	return out
}
