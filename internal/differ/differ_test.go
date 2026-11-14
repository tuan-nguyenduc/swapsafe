package differ

import (
	"net/http"
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func testCase(expect ...api.Assertion) api.TestCase {
	return api.TestCase{
		ID:      "tc1",
		RouteID: "shop/web/shop.example.com/",
		RuleID:  "baseline",
		Expect:  expect,
	}
}

func obs(endpoint string, status int, headers http.Header, hash string) api.Observation {
	return api.Observation{Endpoint: endpoint, Status: status, Headers: headers, BodySHA256: hash}
}

func TestDiffNoDifference(t *testing.T) {
	t.Parallel()

	h := http.Header{"Content-Type": {"text/html"}}
	got := Diff(testCase(),
		obs(api.EndpointBaseline, 200, h, "aaaa"),
		obs(api.EndpointCandidate, 200, h, "aaaa"))

	if len(got) != 0 {
		t.Errorf("identical responses produced findings: %+v", got)
	}
}

func TestDiffStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		base, cand int
		want       api.Severity
	}{
		{name: "2xx against 4xx crosses a class", base: 200, cand: 404, want: api.SeverityCritical},
		{name: "2xx against 3xx crosses a class", base: 200, cand: 301, want: api.SeverityCritical},
		{name: "301 against 308 stays inside 3xx", base: 301, cand: 308, want: api.SeverityMinor},
		{name: "200 against 204 stays inside 2xx", base: 200, cand: 204, want: api.SeverityMinor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Diff(testCase(),
				obs(api.EndpointBaseline, tt.base, nil, "a"),
				obs(api.EndpointCandidate, tt.cand, nil, "a"))

			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1: %+v", len(got), got)
			}
			if got[0].Aspect != api.AspectStatus {
				t.Errorf("aspect = %q, want %q", got[0].Aspect, api.AspectStatus)
			}
			if got[0].Severity != tt.want {
				t.Errorf("severity = %q, want %q", got[0].Severity, tt.want)
			}
			if got[0].Baseline != "200" && got[0].Baseline == got[0].Candidate {
				t.Errorf("finding does not show both values: %+v", got[0])
			}
			if got[0].TestCaseID != "tc1" || got[0].RouteID == "" {
				t.Errorf("finding is not attributable: %+v", got[0])
			}
		})
	}
}

func TestDiffHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		base, cand http.Header
		wantAspect string
		want       api.Severity
	}{
		{
			name:       "a redirect target is always major",
			base:       http.Header{"Location": {"https://shop.example.com/a"}},
			cand:       http.Header{"Location": {"https://shop.example.com/b"}},
			wantAspect: "header:location",
			want:       api.SeverityMajor,
		},
		{
			name:       "a header only one side sends changes behaviour",
			base:       http.Header{"Access-Control-Allow-Origin": {"*"}},
			cand:       nil,
			wantAspect: "header:access-control-allow-origin",
			want:       api.SeverityMajor,
		},
		{
			name:       "a different value is usually cosmetic",
			base:       http.Header{"Content-Type": {"text/html"}},
			cand:       http.Header{"Content-Type": {"text/html; charset=utf-8"}},
			wantAspect: "header:content-type",
			want:       api.SeverityMinor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Diff(testCase(),
				obs(api.EndpointBaseline, 200, tt.base, "a"),
				obs(api.EndpointCandidate, 200, tt.cand, "a"))

			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1: %+v", len(got), got)
			}
			if got[0].Aspect != tt.wantAspect {
				t.Errorf("aspect = %q, want %q", got[0].Aspect, tt.wantAspect)
			}
			if got[0].Severity != tt.want {
				t.Errorf("severity = %q, want %q", got[0].Severity, tt.want)
			}
		})
	}
}

func TestDiffHeaderCaseIsNotADifference(t *testing.T) {
	t.Parallel()

	got := Diff(testCase(),
		obs(api.EndpointBaseline, 200, http.Header{"X-Thing": {"a"}}, "a"),
		obs(api.EndpointCandidate, 200, http.Header{"x-thing": {"a"}}, "a"))

	if len(got) != 0 {
		t.Errorf("header capitalization was reported as a difference: %+v", got)
	}
}

func TestDiffBody(t *testing.T) {
	t.Parallel()

	got := Diff(testCase(),
		obs(api.EndpointBaseline, 200, nil, "aaaaaaaaaaaaaaaa"),
		obs(api.EndpointCandidate, 200, nil, "bbbbbbbbbbbbbbbb"))

	if len(got) != 1 || got[0].Aspect != api.AspectBody {
		t.Fatalf("got %+v, want one body finding", got)
	}
	if got[0].Severity != api.SeverityMajor {
		t.Errorf("severity = %q, want major", got[0].Severity)
	}
	if got[0].Baseline == got[0].Candidate {
		t.Error("both sides show the same hash")
	}
}

func TestDiffTransportErrors(t *testing.T) {
	t.Parallel()

	t.Run("one side down is critical", func(t *testing.T) {
		t.Parallel()

		cand := obs(api.EndpointCandidate, 0, nil, "")
		cand.Err = "dial tcp 203.0.113.20:443: connect: connection refused"

		got := Diff(testCase(), obs(api.EndpointBaseline, 200, nil, "a"), cand)
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1: %+v", len(got), got)
		}
		if got[0].Aspect != api.AspectError || got[0].Severity != api.SeverityCritical {
			t.Errorf("got %+v, want a critical error finding", got[0])
		}
		if got[0].Candidate != cand.Err {
			t.Errorf("the finding does not carry the error: %+v", got[0])
		}
	})

	t.Run("both sides down is not a difference", func(t *testing.T) {
		t.Parallel()

		base := obs(api.EndpointBaseline, 0, nil, "")
		base.Err = "timeout"
		cand := obs(api.EndpointCandidate, 0, nil, "")
		cand.Err = "timeout"

		if got := Diff(testCase(), base, cand); len(got) != 0 {
			t.Errorf("two failures compared as a difference: %+v", got)
		}
	})
}

// A rule that says "for this test, a status difference is only info" wins
// over the default. That is how PLAN 5.7 lets a team accept 308 for 301.
func TestRuleSeverityOverridesTheDefault(t *testing.T) {
	t.Parallel()

	tc := testCase(api.Assertion{Aspect: api.AspectStatus, Severity: api.SeverityInfo})
	got := Diff(tc,
		obs(api.EndpointBaseline, 200, nil, "a"),
		obs(api.EndpointCandidate, 404, nil, "a"))

	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].Severity != api.SeverityInfo {
		t.Errorf("severity = %q, want info: the rule's assertion must win", got[0].Severity)
	}
}

func TestDiffAll(t *testing.T) {
	t.Parallel()

	cases := []api.TestCase{
		{ID: "tc1", RouteID: "r1"},
		{ID: "tc2", RouteID: "r1"},
		{ID: "tc3", RouteID: "r2"}, // never probed
	}
	observations := map[string][]api.Observation{
		"tc1": {obs(api.EndpointCandidate, 404, nil, "a"), obs(api.EndpointBaseline, 200, nil, "a")},
		"tc2": {obs(api.EndpointBaseline, 200, nil, "a"), obs(api.EndpointCandidate, 200, nil, "a")},
		"tc3": {obs(api.EndpointBaseline, 200, nil, "a")}, // only one side
	}

	got := DiffAll(cases, observations)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	if got[0].TestCaseID != "tc1" {
		t.Errorf("finding is for %q, want tc1", got[0].TestCaseID)
	}
	// The observations arrived candidate-first; the finding must not.
	if got[0].Baseline != "200" || got[0].Candidate != "404" {
		t.Errorf("baseline and candidate are swapped: %+v", got[0])
	}
}

func TestDiffIsDeterministic(t *testing.T) {
	t.Parallel()

	base := obs(api.EndpointBaseline, 200, http.Header{
		"A-Header": {"1"}, "B-Header": {"2"}, "C-Header": {"3"}, "Location": {"/a"},
	}, "aaaa")
	cand := obs(api.EndpointCandidate, 200, http.Header{
		"A-Header": {"9"}, "B-Header": {"8"}, "C-Header": {"7"}, "Location": {"/b"},
	}, "bbbb")

	first := Diff(testCase(), base, cand)
	if len(first) < 4 {
		t.Fatalf("got %d findings, want at least 4", len(first))
	}
	for range 10 {
		next := Diff(testCase(), base, cand)
		for i := range first {
			if next[i].Aspect != first[i].Aspect {
				t.Fatalf("finding %d changed order between runs: map iteration is leaking", i)
			}
		}
	}
}
