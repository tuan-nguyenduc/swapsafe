package v1alpha1

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tuan-nguyenduc/swapsafe/internal/model"
)

// sampleRun returns one fully populated run: every field of every type set to
// a non-zero value, so a field added without a JSON tag shows up as a
// round-trip failure rather than silently vanishing from results.json.
func sampleRun() (Metadata, []model.RouteSpec, []model.TestCase, map[string][]model.Observation, []model.Finding) {
	target := model.ObjectRef{
		Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web",
	}
	source := model.ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"}

	meta := Metadata{
		GeneratedAt: time.Date(2026, time.November, 14, 9, 30, 0, 0, time.UTC),
		Version:     "v0.1.0",
		Baseline:    "https://203.0.113.10",
		Candidate:   "https://203.0.113.20",
	}

	routes := []model.RouteSpec{{
		ID:          "shop/web/shop.example.com/",
		Host:        "shop.example.com",
		Path:        "/",
		PathType:    model.PathTypePrefix,
		TLS:         true,
		Annotations: map[string]string{"nginx.ingress.kubernetes.io/ssl-redirect": "true"},
		Source:      source,
		Target:      &target,
	}}

	cases := []model.TestCase{{
		ID:      "c0ffee",
		RouteID: routes[0].ID,
		RuleID:  "nginx.ssl-redirect",
		Request: model.RequestSpec{
			Method:  http.MethodGet,
			Scheme:  "http",
			Host:    "shop.example.com",
			Path:    "/checkout",
			Query:   "ref=email",
			Headers: http.Header{"Accept": []string{"text/html"}},
			Body:    []byte("ignored for GET"),
		},
		Expect: []model.Assertion{
			{Aspect: "status", Severity: model.SeverityCritical},
			{Aspect: "header:location"},
		},
	}}

	observations := map[string][]model.Observation{
		"c0ffee": {
			{
				Endpoint:   model.EndpointBaseline,
				Status:     http.StatusMovedPermanently,
				Headers:    http.Header{"Location": []string{"https://shop.example.com/checkout"}},
				BodySHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				BodySample: []byte("<html>"),
				LatencyMS:  12,
			},
			{
				Endpoint:  model.EndpointCandidate,
				Status:    http.StatusFound,
				Headers:   http.Header{"Location": []string{"https://shop.example.com/checkout"}},
				LatencyMS: 9,
				Err:       "",
			},
		},
	}

	findings := []model.Finding{{
		RouteID:    routes[0].ID,
		TestCaseID: "c0ffee",
		Severity:   model.SeverityMinor,
		Aspect:     "status",
		Baseline:   "301",
		Candidate:  "302",
		Attribution: []model.Cause{{
			Annotation: "nginx.ingress.kubernetes.io/ssl-redirect",
			Value:      "true",
			Object:     source,
		}},
		Hint: "Set a RequestRedirect filter with statusCode: 301.",
	}}

	return meta, routes, cases, observations, findings
}

func TestResultsJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want := NewResults(sampleRun())

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Results
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the document:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestResultsCarriesAPIVersion(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(NewResults(sampleRun()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var envelope struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.APIVersion != APIVersion {
		t.Errorf("apiVersion = %q, want %q", envelope.APIVersion, APIVersion)
	}
	if envelope.Kind != KindResults {
		t.Errorf("kind = %q, want %q", envelope.Kind, KindResults)
	}
}

// PLAN section 3: the same input must produce the same bytes, so that reports
// from two runs can be diffed and golden files stay stable.
func TestResultsMarshalIsDeterministic(t *testing.T) {
	t.Parallel()

	first, err := json.Marshal(NewResults(sampleRun()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for range 10 {
		next, err := json.Marshal(NewResults(sampleRun()))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(next) != string(first) {
			t.Fatalf("marshal is not deterministic:\nfirst: %s\n next: %s", first, next)
		}
	}
}

func TestNewResultsDefaultsGeneratedAt(t *testing.T) {
	t.Parallel()

	got := NewResults(Metadata{}, nil, nil, nil, nil)
	if got.Metadata.GeneratedAt.IsZero() {
		t.Error("GeneratedAt was left zero")
	}
	if loc := got.Metadata.GeneratedAt.Location(); loc != time.UTC {
		t.Errorf("GeneratedAt location = %v, want UTC", loc)
	}
	if got.Routes != nil || got.TestCases != nil || got.Observations != nil || got.Findings != nil {
		t.Error("empty input should leave the collections nil, not empty slices")
	}
}

func TestNewResultsConvertsEveryField(t *testing.T) {
	t.Parallel()

	meta, routes, cases, observations, findings := sampleRun()
	got := NewResults(meta, routes, cases, observations, findings)

	route := got.Routes[0]
	if route.ID != routes[0].ID || route.PathType != model.PathTypePrefix || !route.TLS {
		t.Errorf("route converted wrong: %+v", route)
	}
	if route.Target == nil || route.Target.Name != "web" {
		t.Errorf("target reference lost: %+v", route.Target)
	}

	tc := got.TestCases[0]
	if tc.Request.Query != "ref=email" || tc.Request.Headers["Accept"][0] != "text/html" {
		t.Errorf("request converted wrong: %+v", tc.Request)
	}
	if tc.Expect[0].Severity != string(model.SeverityCritical) {
		t.Errorf("assertion severity = %q", tc.Expect[0].Severity)
	}
	if tc.Expect[1].Severity != "" {
		t.Errorf("unset assertion severity became %q, want empty", tc.Expect[1].Severity)
	}

	if got.Findings[0].Attribution[0].Object.Kind != "Ingress" {
		t.Errorf("attribution lost its object: %+v", got.Findings[0].Attribution)
	}
	if got.Observations["c0ffee"][1].Error != "" {
		t.Errorf("unexpected error on the candidate observation")
	}
}

// A route with no matching HTTPRoute is the critical case from M1-T2; its
// target must stay absent rather than serialize as a zero object.
func TestRouteWithoutTargetOmitsIt(t *testing.T) {
	t.Parallel()

	res := NewResults(Metadata{}, []model.RouteSpec{{ID: "shop/web/x/", Source: model.ObjectRef{Kind: "Ingress", Name: "web"}}}, nil, nil, nil)
	if res.Routes[0].Target != nil {
		t.Fatalf("Target = %+v, want nil", res.Routes[0].Target)
	}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"target"`) {
		t.Errorf("absent target was serialized: %s", data)
	}
}
