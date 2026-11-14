package report

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func results(findings ...api.Finding) api.Results {
	res := api.NewResults(api.Metadata{Version: "v0.1.0"})
	res.Routes = []api.RouteSpec{{ID: "shop/web/h/"}}
	res.TestCases = []api.TestCase{{
		ID:      "tc1",
		RouteID: "shop/web/h/",
		Request: api.RequestSpec{Method: "GET", Scheme: "http", Path: "/"},
	}}
	res.Findings = findings
	return res
}

func TestTableNoFindings(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	if err := Table(&out, results()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No differences") {
		t.Errorf("a clean run must say so plainly, got:\n%s", out.String())
	}
}

func TestTableOrdersBySeverity(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	err := Table(&out, results(
		api.Finding{RouteID: "shop/web/h/", Severity: api.SeverityInfo, Aspect: "annotation"},
		api.Finding{RouteID: "shop/web/h/", Severity: api.SeverityCritical, Aspect: api.AspectStatus, TestCaseID: "tc1"},
		api.Finding{RouteID: "shop/web/h/", Severity: api.SeverityMinor, Aspect: "header:x"},
	))
	if err != nil {
		t.Fatal(err)
	}

	body := out.String()
	critical, minor, info := strings.Index(body, "critical"), strings.Index(body, "minor"), strings.Index(body, "info")
	if critical >= minor || minor >= info {
		t.Errorf("findings are not most-severe-first:\n%s", body)
	}
	if !strings.Contains(body, "3 findings: 1 critical, 1 minor, 1 info.") {
		t.Errorf("summary line is wrong:\n%s", body)
	}
}

// Two test cases on one route can fail identically; the table has to say
// which request each row came from.
func TestTableNamesTheRequest(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	err := Table(&out, results(
		api.Finding{RouteID: "shop/web/h/", TestCaseID: "tc1", Severity: api.SeverityCritical, Aspect: api.AspectStatus},
		api.Finding{RouteID: "shop/web/h/", Severity: api.SeverityCritical, Aspect: api.AspectRoute},
	))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "GET http /") {
		t.Errorf("the request is missing from the table:\n%s", out.String())
	}
	// A route finding had no request, and must not borrow one.
	if strings.Count(out.String(), "GET http /") != 1 {
		t.Errorf("a finding with no test case was given a request:\n%s", out.String())
	}
}

func TestTableDeduplicatesHints(t *testing.T) {
	t.Parallel()

	const hint = "set a RequestRedirect filter"
	var out strings.Builder
	err := Table(&out, results(
		api.Finding{RouteID: "a", Severity: api.SeverityMajor, Aspect: "header:location", Hint: hint},
		api.Finding{RouteID: "b", Severity: api.SeverityMajor, Aspect: "header:location", Hint: hint},
	))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), hint); n != 1 {
		t.Errorf("hint printed %d times, want 1: repeating it is how a report stops being read", n)
	}
}

func TestJSONIsReadableAndVersioned(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	if err := JSON(&out, results(api.Finding{RouteID: "a", Severity: api.SeverityMajor})); err != nil {
		t.Fatal(err)
	}

	var back api.Results
	if err := json.Unmarshal([]byte(out.String()), &back); err != nil {
		t.Fatalf("output is not valid json: %v", err)
	}
	if back.APIVersion != api.APIVersion {
		t.Errorf("apiVersion = %q, want %q", back.APIVersion, api.APIVersion)
	}
	if !strings.Contains(out.String(), "\n  ") {
		t.Error("json is not indented; it has to survive review in a diff")
	}
}

func TestWriteRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	err := Write(&out, "yaml", results())
	if err == nil {
		t.Fatal("an unknown format was accepted")
	}
	if !strings.Contains(err.Error(), "table") {
		t.Errorf("the error does not list what is supported: %v", err)
	}
}

func TestScanTable(t *testing.T) {
	t.Parallel()

	routes := []api.RouteSpec{
		{ID: "shop/web/h/", Target: &api.ObjectRef{Kind: "HTTPRoute", Namespace: "shop", Name: "web"}},
		{ID: "shop/admin/h/"},
	}
	rules := func(api.RouteSpec) []string { return []string{"baseline"} }

	var out strings.Builder
	if err := ScanTable(&out, routes, rules, nil); err != nil {
		t.Fatal(err)
	}

	body := out.String()
	if !strings.Contains(body, "HTTPRoute shop/web") {
		t.Errorf("the paired target is missing:\n%s", body)
	}
	if !strings.Contains(body, "MISSING") {
		t.Errorf("an unpaired route must stand out:\n%s", body)
	}
	if !strings.Contains(body, "nothing to flag before probing") {
		t.Errorf("a clean scan must say so:\n%s", body)
	}
}

func TestTruncateKeepsTheColumnReadable(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 200)
	var out strings.Builder
	if err := Table(&out, results(api.Finding{RouteID: "a", Severity: api.SeverityMajor, Baseline: long})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), long) {
		t.Error("a 200 character value was printed in full")
	}
	if !strings.Contains(out.String(), "…") {
		t.Error("truncation is not marked")
	}
}
