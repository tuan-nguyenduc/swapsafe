package v1alpha1

import (
	"net/http"
	"time"

	"github.com/tuan-nguyenduc/swapsafe/internal/model"
)

// NewResults assembles a results document from one run. Slices are converted
// in order, so the same run always produces byte-identical JSON.
func NewResults(
	meta Metadata,
	routes []model.RouteSpec,
	cases []model.TestCase,
	observations map[string][]model.Observation,
	findings []model.Finding,
) Results {
	if meta.GeneratedAt.IsZero() {
		meta.GeneratedAt = time.Now().UTC()
	}
	meta.GeneratedAt = meta.GeneratedAt.UTC()

	res := Results{
		APIVersion: APIVersion,
		Kind:       KindResults,
		Metadata:   meta,
	}
	for _, r := range routes {
		res.Routes = append(res.Routes, FromRouteSpec(r))
	}
	for _, tc := range cases {
		res.TestCases = append(res.TestCases, FromTestCase(tc))
	}
	if len(observations) > 0 {
		res.Observations = make(map[string][]Observation, len(observations))
		for id, obs := range observations {
			converted := make([]Observation, 0, len(obs))
			for _, o := range obs {
				converted = append(converted, FromObservation(o))
			}
			res.Observations[id] = converted
		}
	}
	for _, f := range findings {
		res.Findings = append(res.Findings, FromFinding(f))
	}
	return res
}

// FromObjectRef converts an internal object reference.
func FromObjectRef(o model.ObjectRef) ObjectRef {
	return ObjectRef{Group: o.Group, Kind: o.Kind, Namespace: o.Namespace, Name: o.Name}
}

// FromRouteSpec converts an internal route.
func FromRouteSpec(r model.RouteSpec) Route {
	out := Route{
		ID:          r.ID,
		Host:        r.Host,
		Path:        r.Path,
		PathType:    r.PathType,
		TLS:         r.TLS,
		Annotations: r.Annotations,
		Source:      FromObjectRef(r.Source),
	}
	if r.Target != nil {
		target := FromObjectRef(*r.Target)
		out.Target = &target
	}
	return out
}

// FromTestCase converts an internal test case.
func FromTestCase(tc model.TestCase) TestCase {
	out := TestCase{
		ID:      tc.ID,
		RouteID: tc.RouteID,
		RuleID:  tc.RuleID,
		Request: Request{
			Method:  tc.Request.Method,
			Scheme:  tc.Request.Scheme,
			Host:    tc.Request.Host,
			Path:    tc.Request.Path,
			Query:   tc.Request.Query,
			Headers: fromHeader(tc.Request.Headers),
			Body:    tc.Request.Body,
		},
	}
	for _, a := range tc.Expect {
		out.Expect = append(out.Expect, Assertion{Aspect: a.Aspect, Severity: a.Severity.String()})
	}
	return out
}

// FromObservation converts an internal observation.
func FromObservation(o model.Observation) Observation {
	return Observation{
		Endpoint:   o.Endpoint,
		Status:     o.Status,
		Headers:    fromHeader(o.Headers),
		BodySHA256: o.BodySHA256,
		BodySample: o.BodySample,
		LatencyMS:  o.LatencyMS,
		Error:      o.Err,
	}
}

// FromFinding converts an internal finding.
func FromFinding(f model.Finding) Finding {
	out := Finding{
		RouteID:    f.RouteID,
		TestCaseID: f.TestCaseID,
		Severity:   f.Severity.String(),
		Aspect:     f.Aspect,
		Baseline:   f.Baseline,
		Candidate:  f.Candidate,
		Hint:       f.Hint,
	}
	for _, c := range f.Attribution {
		out.Attribution = append(out.Attribution, Cause{
			Annotation: c.Annotation,
			Value:      c.Value,
			Object:     FromObjectRef(c.Object),
		})
	}
	return out
}

func fromHeader(h http.Header) map[string][]string {
	if len(h) == 0 {
		return nil
	}
	return h
}
