// Package model holds the core swapsafe data types. It is a leaf package: it
// imports nothing else from this repository, so every other package can depend
// on it without creating a cycle (PLAN section 4).
package model

import "net/http"

// Path types as they appear on an Ingress rule.
const (
	PathTypeExact                  = "Exact"
	PathTypePrefix                 = "Prefix"
	PathTypeImplementationSpecific = "ImplementationSpecific"
)

// The two endpoints every test case is sent to.
const (
	EndpointBaseline  = "baseline"
	EndpointCandidate = "candidate"
)

// ObjectRef identifies a Kubernetes object a route or a finding came from.
type ObjectRef struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

// String renders the reference the way kubectl would print it.
func (o ObjectRef) String() string {
	kind := o.Kind
	if o.Group != "" {
		kind += "." + o.Group
	}
	if o.Namespace == "" {
		return kind + " " + o.Name
	}
	return kind + " " + o.Namespace + "/" + o.Name
}

// RouteSpec is one logical route, normalized from the source Ingress and,
// where one was found, the Gateway API HTTPRoute meant to replace it.
type RouteSpec struct {
	// ID is "<namespace>/<ingress>/<host><path>". It is stable across runs so
	// that reports from different runs can be compared.
	ID          string
	Host        string
	Path        string
	PathType    string
	TLS         bool
	Annotations map[string]string
	Source      ObjectRef
	// Target is nil when no HTTPRoute could be matched to this route.
	Target *ObjectRef
}

// RequestSpec is the request a test case sends to both endpoints. The host
// here is the route's host: the prober connects to the endpoint address but
// sets Host and SNI from this field, so routes can be tested before DNS moves.
type RequestSpec struct {
	Method  string
	Scheme  string // http or https
	Host    string
	Path    string
	Query   string
	Headers http.Header
	Body    []byte
}

// Assertion names an aspect of the response that a rule cares about, and the
// severity to report when the two endpoints disagree on it. An empty Severity
// means the differ picks the default for that aspect.
type Assertion struct {
	Aspect   string // "status", "header:location", "body", ...
	Severity Severity
}

// TestCase is one request to send to both endpoints and compare.
type TestCase struct {
	// ID is a stable hash of the route, the rule and the variant, so the same
	// input always produces the same test case ID.
	ID      string
	RouteID string
	RuleID  string // "nginx.ssl-redirect"
	Request RequestSpec
	Expect  []Assertion
}

// Observation is one endpoint's response to a test case, after normalization.
type Observation struct {
	Endpoint   string // EndpointBaseline or EndpointCandidate
	Status     int
	Headers    http.Header
	BodySHA256 string
	// BodySample holds at most 4 KiB and is only populated when the endpoints
	// differ and --include-bodies was passed.
	BodySample []byte
	LatencyMS  int64
	// Err is set when the request failed outright (network, TLS, timeout).
	Err string
}

// Cause is an annotation, and the object carrying it, that explains a finding.
type Cause struct {
	Annotation string // "nginx.ingress.kubernetes.io/rewrite-target"
	Value      string
	Object     ObjectRef
}

// Finding is one meaningful difference between the two endpoints.
type Finding struct {
	TestCaseID  string
	Severity    Severity
	Aspect      string
	Baseline    string
	Candidate   string
	Attribution []Cause
	Hint        string
}
