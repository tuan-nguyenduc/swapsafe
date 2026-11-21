// Package v1alpha1 holds swapsafe's core types. They are both what the
// pipeline passes between stages and, with their JSON tags, the exact shape
// of results.json. Rules live outside this module's internal tree, so the
// types their interface mentions have to be importable from anywhere.
//
// Within one apiVersion these types only change in backwards compatible ways
// (PLAN section 5.1).
package v1alpha1

import (
	"net/http"
	"strings"
	"time"
)

// APIVersion identifies this schema. It appears in every document swapsafe writes.
const APIVersion = "swapsafe.dev/v1alpha1"

// KindResults is the kind of a results document.
const KindResults = "Results"

// Path types as they appear on an Ingress rule.
const (
	PathTypeExact                  = "Exact"
	PathTypePrefix                 = "Prefix"
	PathTypeImplementationSpecific = "ImplementationSpecific"
)

// The parts of a response, or of the configuration, a finding can be about.
const (
	// AspectRoute: the Ingress route has no counterpart on the Gateway side.
	AspectRoute = "route"
	// AspectAnnotation: an annotation no rule verifies.
	AspectAnnotation = "annotation"
	// AspectError: one endpoint failed to answer at all.
	AspectError = "error"
	// AspectStatus: the two endpoints answered with different status codes.
	AspectStatus = "status"
	// AspectBody: same status, different body.
	AspectBody = "body"
	// HeaderAspectPrefix starts every per-header aspect, as in "header:location".
	HeaderAspectPrefix = "header:"
)

// HeaderAspect names the aspect for one response header. The name is
// lowercased so an aspect never depends on how a proxy capitalized it.
func HeaderAspect(name string) string {
	return HeaderAspectPrefix + strings.ToLower(name)
}

// ValueAbsent is what a finding shows for a side that sent nothing at all —
// no such header, no body. The differ writes it and the report dims it.
const ValueAbsent = "(absent)"

// The two endpoints every test case is sent to.
const (
	EndpointBaseline  = "baseline"
	EndpointCandidate = "candidate"
)

// Results is the whole output of one run. It carries everything needed to
// re-render any report format without probing again (PLAN section 3).
type Results struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   Metadata    `json:"metadata"`
	Routes     []RouteSpec `json:"routes"`
	TestCases  []TestCase  `json:"testCases"`
	// Observations maps a test case ID to the responses from both endpoints.
	Observations map[string][]Observation `json:"observations"`
	Findings     []Finding                `json:"findings"`
}

// NewResults stamps the schema fields on a results document and defaults the
// timestamp, so no caller has to remember either.
func NewResults(meta Metadata) Results {
	if meta.GeneratedAt.IsZero() {
		meta.GeneratedAt = time.Now()
	}
	meta.GeneratedAt = meta.GeneratedAt.UTC()

	return Results{APIVersion: APIVersion, Kind: KindResults, Metadata: meta}
}

// Metadata records what was run, when, and against what.
type Metadata struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Version     string    `json:"version"`
	Baseline    string    `json:"baseline"`
	Candidate   string    `json:"candidate"`
}

// ObjectRef identifies a Kubernetes object a route or a finding came from.
type ObjectRef struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
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
	ID          string            `json:"id"`
	Host        string            `json:"host"`
	Path        string            `json:"path"`
	PathType    string            `json:"pathType"`
	TLS         bool              `json:"tls"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Source      ObjectRef         `json:"source"`
	// Target is nil when no HTTPRoute could be matched to this route.
	Target *ObjectRef `json:"target,omitempty"`
}

// RequestSpec is the request a test case sends to both endpoints. The host
// here is the route's host: the prober connects to the endpoint address but
// sets Host and SNI from this field, so routes can be tested before DNS moves.
type RequestSpec struct {
	Method  string      `json:"method"`
	Scheme  string      `json:"scheme"`
	Host    string      `json:"host"`
	Path    string      `json:"path"`
	Query   string      `json:"query,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Body    []byte      `json:"body,omitempty"`
}

// Assertion names an aspect of the response that a rule cares about, and the
// severity to report when the two endpoints disagree on it. An empty Severity
// means the differ picks the default for that aspect.
type Assertion struct {
	Aspect   string   `json:"aspect"` // "status", "header:location", "body", ...
	Severity Severity `json:"severity,omitempty"`
}

// TestCase is one request to send to both endpoints and compare.
type TestCase struct {
	// ID is a stable hash of the route, the rule and the variant, so the same
	// input always produces the same test case ID.
	ID      string      `json:"id"`
	RouteID string      `json:"routeId"`
	RuleID  string      `json:"ruleId"` // "nginx.ssl-redirect"
	Request RequestSpec `json:"request"`
	Expect  []Assertion `json:"expect,omitempty"`
}

// Observation is one endpoint's response to a test case, after normalization.
type Observation struct {
	Endpoint string      `json:"endpoint"` // EndpointBaseline or EndpointCandidate
	Status   int         `json:"status"`
	Headers  http.Header `json:"headers,omitempty"`
	// BodySHA256 is the digest the two endpoints were compared on. It is the
	// digest of the body, unless fields had to be excluded to compare at all,
	// in which case BodyIgnoredFields names them.
	BodySHA256 string `json:"bodySha256,omitempty"`
	// BodyIgnoredFields are the JSON paths left out of the comparison because
	// the baseline changed them on its own between two samples.
	BodyIgnoredFields []string `json:"bodyIgnoredFields,omitempty"`
	// BodySample holds at most 4 KiB and is only populated when the endpoints
	// differ and --include-bodies was passed.
	BodySample []byte `json:"bodySample,omitempty"`
	LatencyMS  int64  `json:"latencyMs"`
	// BodyUnstable means the baseline did not return the same body twice for
	// this request, so the two sides cannot be compared on it. Plenty of
	// applications put a counter, a timestamp or a pod name in every response.
	BodyUnstable bool `json:"bodyUnstable,omitempty"`
	// Err is set when the request failed outright (network, TLS, timeout).
	Err string `json:"error,omitempty"`
}

// Cause is an annotation, and the object carrying it, that explains a finding.
type Cause struct {
	Annotation string    `json:"annotation,omitempty"` // "nginx.ingress.kubernetes.io/rewrite-target"
	Value      string    `json:"value,omitempty"`
	Object     ObjectRef `json:"object"`
}

// Finding is one meaningful difference between the two endpoints.
type Finding struct {
	// RouteID is always set. TestCaseID is empty for findings that belong to a
	// route rather than to a request — an Ingress route with no matching
	// HTTPRoute, or an annotation no rule verifies.
	RouteID     string   `json:"routeId"`
	TestCaseID  string   `json:"testCaseId,omitempty"`
	Severity    Severity `json:"severity"`
	Aspect      string   `json:"aspect"`
	Baseline    string   `json:"baseline"`
	Candidate   string   `json:"candidate"`
	Attribution []Cause  `json:"attribution,omitempty"`
	Hint        string   `json:"hint,omitempty"`
}
