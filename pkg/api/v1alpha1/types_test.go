package v1alpha1

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sampleResults returns a fully populated document: every field of every type
// set to a non-zero value, so a field added without a JSON tag shows up as a
// round-trip failure rather than silently vanishing from results.json.
func sampleResults() Results {
	source := ObjectRef{Kind: "Ingress", Namespace: "shop", Name: "web"}
	target := ObjectRef{
		Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "web",
	}

	res := NewResults(Metadata{
		GeneratedAt: time.Date(2026, time.November, 15, 9, 30, 0, 0, time.UTC),
		Version:     "v0.1.0",
		Baseline:    "https://203.0.113.10",
		Candidate:   "https://203.0.113.20",
	})

	res.Routes = []RouteSpec{{
		ID:          "shop/web/shop.example.com/",
		Host:        "shop.example.com",
		Path:        "/",
		PathType:    PathTypePrefix,
		TLS:         true,
		Annotations: map[string]string{"nginx.ingress.kubernetes.io/ssl-redirect": "true"},
		Source:      source,
		Target:      &target,
	}}

	res.TestCases = []TestCase{{
		ID:      "c0ffee",
		RouteID: res.Routes[0].ID,
		RuleID:  "nginx.ssl-redirect",
		Request: RequestSpec{
			Method:  http.MethodGet,
			Scheme:  "http",
			Host:    "shop.example.com",
			Path:    "/checkout",
			Query:   "ref=email",
			Headers: http.Header{"Accept": []string{"text/html"}},
			Body:    []byte("ignored for GET"),
		},
		Expect: []Assertion{
			{Aspect: "status", Severity: SeverityCritical},
			{Aspect: "header:location"},
		},
	}}

	res.Observations = map[string][]Observation{"c0ffee": {
		{
			Endpoint:   EndpointBaseline,
			Status:     http.StatusMovedPermanently,
			Headers:    http.Header{"Location": []string{"https://shop.example.com/checkout"}},
			BodySHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			BodySample: []byte("<html>"),
			LatencyMS:  12,
		},
		{
			Endpoint:  EndpointCandidate,
			Status:    http.StatusFound,
			Headers:   http.Header{"Location": []string{"https://shop.example.com/checkout"}},
			LatencyMS: 9,
		},
	}}

	res.Findings = []Finding{{
		RouteID:    res.Routes[0].ID,
		TestCaseID: "c0ffee",
		Severity:   SeverityMinor,
		Aspect:     "status",
		Baseline:   "301",
		Candidate:  "302",
		Attribution: []Cause{{
			Annotation: "nginx.ingress.kubernetes.io/ssl-redirect",
			Value:      "true",
			Object:     source,
		}},
		Hint: "Set a RequestRedirect filter with statusCode: 301.",
	}}

	return res
}

func TestResultsJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want := sampleResults()

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

	data, err := json.Marshal(sampleResults())
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

	first, err := json.Marshal(sampleResults())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for range 10 {
		next, err := json.Marshal(sampleResults())
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

	got := NewResults(Metadata{})
	if got.Metadata.GeneratedAt.IsZero() {
		t.Error("GeneratedAt was left zero")
	}
	if loc := got.Metadata.GeneratedAt.Location(); loc != time.UTC {
		t.Errorf("GeneratedAt location = %v, want UTC", loc)
	}
	if got.APIVersion != APIVersion || got.Kind != KindResults {
		t.Errorf("schema fields not stamped: %+v", got)
	}
}

// A route with no matching HTTPRoute is the critical case from M1-T2; its
// target must stay absent rather than serialize as a zero object.
func TestRouteWithoutTargetOmitsIt(t *testing.T) {
	t.Parallel()

	res := NewResults(Metadata{})
	res.Routes = []RouteSpec{{ID: "shop/web/x/", Source: ObjectRef{Kind: "Ingress", Name: "web"}}}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"target"`) {
		t.Errorf("absent target was serialized: %s", data)
	}
}
