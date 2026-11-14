// Package v1alpha1 is the public, versioned shape of everything swapsafe
// writes to results.json. These are JSON-tagged copies of the internal model
// types: internal/model is free to change, this package is not. Within one
// apiVersion, changes here must stay backwards compatible (PLAN section 5.1).
package v1alpha1

import "time"

// APIVersion identifies this schema. It appears in every document swapsafe writes.
const APIVersion = "swapsafe.dev/v1alpha1"

// KindResults is the kind of a results document.
const KindResults = "Results"

// Results is the whole output of one run. It carries everything needed to
// re-render any report format without probing again (PLAN section 3).
type Results struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   Metadata   `json:"metadata"`
	Routes     []Route    `json:"routes"`
	TestCases  []TestCase `json:"testCases"`
	// Observations maps a test case ID to the responses from both endpoints.
	Observations map[string][]Observation `json:"observations"`
	Findings     []Finding                `json:"findings"`
}

// Metadata records what was run, when, and against what.
type Metadata struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Version     string    `json:"version"`
	Baseline    string    `json:"baseline"`
	Candidate   string    `json:"candidate"`
}

// ObjectRef identifies a Kubernetes object.
type ObjectRef struct {
	Group     string `json:"group,omitempty"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// Route is one logical route, from the source Ingress to its HTTPRoute.
type Route struct {
	ID          string            `json:"id"`
	Host        string            `json:"host"`
	Path        string            `json:"path"`
	PathType    string            `json:"pathType"`
	TLS         bool              `json:"tls"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Source      ObjectRef         `json:"source"`
	Target      *ObjectRef        `json:"target,omitempty"`
}

// Request is the request a test case sends to both endpoints.
type Request struct {
	Method  string              `json:"method"`
	Scheme  string              `json:"scheme"`
	Host    string              `json:"host"`
	Path    string              `json:"path"`
	Query   string              `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// Assertion names a response aspect a rule compares, and how bad a mismatch is.
type Assertion struct {
	Aspect   string `json:"aspect"`
	Severity string `json:"severity,omitempty"`
}

// TestCase is one request sent to both endpoints.
type TestCase struct {
	ID      string      `json:"id"`
	RouteID string      `json:"routeId"`
	RuleID  string      `json:"ruleId"`
	Request Request     `json:"request"`
	Expect  []Assertion `json:"expect,omitempty"`
}

// Observation is one endpoint's normalized response.
type Observation struct {
	Endpoint   string              `json:"endpoint"`
	Status     int                 `json:"status"`
	Headers    map[string][]string `json:"headers,omitempty"`
	BodySHA256 string              `json:"bodySha256,omitempty"`
	BodySample []byte              `json:"bodySample,omitempty"`
	LatencyMS  int64               `json:"latencyMs"`
	Error      string              `json:"error,omitempty"`
}

// Cause is an annotation, and the object carrying it, that explains a finding.
type Cause struct {
	Annotation string    `json:"annotation"`
	Value      string    `json:"value,omitempty"`
	Object     ObjectRef `json:"object"`
}

// Finding is one meaningful difference between the two endpoints.
type Finding struct {
	TestCaseID  string  `json:"testCaseId"`
	Severity    string  `json:"severity"`
	Aspect      string  `json:"aspect"`
	Baseline    string  `json:"baseline"`
	Candidate   string  `json:"candidate"`
	Attribution []Cause `json:"attribution,omitempty"`
	Hint        string  `json:"hint,omitempty"`
}
