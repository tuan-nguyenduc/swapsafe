// Package report renders a finished run. Every format is built from the same
// Results document, so `swapsafe report` can re-render an old run without
// probing anything again (PLAN section 3).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// Formats are the values --output accepts.
const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Formats lists every supported --output value, for help text and validation.
func Formats() []string { return []string{FormatTable, FormatJSON} }

// Write renders res in the named format.
func Write(w io.Writer, format string, res api.Results) error {
	switch format {
	case FormatTable:
		return Table(w, res)
	case FormatJSON:
		return JSON(w, res)
	default:
		return fmt.Errorf("unknown output format %q: want %s", format, strings.Join(Formats(), ", "))
	}
}

// JSON writes the results document. This is the format every other one is
// rendered from, so it is indented to stay reviewable in a diff.
func JSON(w io.Writer, res api.Results) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		return fmt.Errorf("writing json: %w", err)
	}
	return nil
}

// Table writes the findings for a human: most severe first, then the hints,
// each one printed once however many findings share it.
func Table(w io.Writer, res api.Results) error {
	if len(res.Findings) == 0 {
		_, err := fmt.Fprintf(w, "No differences across %d routes and %d test cases.\n",
			len(res.Routes), len(res.TestCases))
		return err
	}

	findings := slices.Clone(res.Findings)
	slices.SortStableFunc(findings, bySeverityThenRoute)

	// Two test cases on the same route can differ in the same way, so the
	// table has to say which request produced each row.
	requests := make(map[string]string, len(res.TestCases))
	for _, tc := range res.TestCases {
		requests[tc.ID] = describeRequest(tc.Request)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SEVERITY\tROUTE\tREQUEST\tASPECT\tBASELINE\tCANDIDATE")
	for _, f := range findings {
		request, ok := requests[f.TestCaseID]
		if !ok {
			// A route or annotation finding: nothing was sent for it.
			request = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			f.Severity, f.RouteID, request, f.Aspect, truncate(f.Baseline), truncate(f.Candidate))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("writing table: %w", err)
	}

	if _, err := fmt.Fprintf(w, "\n%s\n", summarize(findings)); err != nil {
		return err
	}
	return writeHints(w, findings)
}

// ScanTable writes what `swapsafe scan` found without sending any request:
// the routes, where each one is meant to land, and which rules will probe it.
func ScanTable(w io.Writer, routes []api.RouteSpec, rulesFor func(api.RouteSpec) []string, findings []api.Finding) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ROUTE\tTARGET\tRULES")
	for _, r := range routes {
		target := "MISSING"
		if r.Target != nil {
			target = r.Target.Kind + " " + r.Target.Namespace + "/" + r.Target.Name
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.ID, target, strings.Join(rulesFor(r), ", "))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("writing table: %w", err)
	}

	if len(findings) == 0 {
		_, err := fmt.Fprintf(w, "\n%d routes, nothing to flag before probing.\n", len(routes))
		return err
	}
	if _, err := fmt.Fprintf(w, "\n%d routes. %s\n", len(routes), summarize(findings)); err != nil {
		return err
	}
	return writeHints(w, findings)
}

// bySeverityThenRoute puts the findings a user must act on at the top, and
// keeps the order stable for everything that ties.
func bySeverityThenRoute(a, b api.Finding) int {
	if d := b.Severity.Rank() - a.Severity.Rank(); d != 0 {
		return d
	}
	if d := strings.Compare(a.RouteID, b.RouteID); d != 0 {
		return d
	}
	if d := strings.Compare(a.TestCaseID, b.TestCaseID); d != 0 {
		return d
	}
	return strings.Compare(a.Aspect, b.Aspect)
}

func summarize(findings []api.Finding) string {
	counts := map[api.Severity]int{}
	for _, f := range findings {
		counts[f.Severity]++
	}

	var parts []string
	for _, s := range []api.Severity{api.SeverityCritical, api.SeverityMajor, api.SeverityMinor, api.SeverityInfo} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return fmt.Sprintf("%s: %s.", plural(len(findings), "finding"), strings.Join(parts, ", "))
}

// writeHints prints each distinct hint once. Repeating the same sentence
// under forty findings is how a report stops being read.
func writeHints(w io.Writer, findings []api.Finding) error {
	var hints []string
	for _, f := range findings {
		if f.Hint != "" && !slices.Contains(hints, f.Hint) {
			hints = append(hints, f.Hint)
		}
	}
	if len(hints) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(w, "\nHints:"); err != nil {
		return err
	}
	for i, h := range hints {
		if _, err := fmt.Fprintf(w, "  %d. %s\n", i+1, h); err != nil {
			return err
		}
	}
	return nil
}

// describeRequest is the short form of a request for a table cell.
func describeRequest(r api.RequestSpec) string {
	out := r.Method + " " + r.Scheme + " " + r.Path
	if r.Query != "" {
		out += "?" + r.Query
	}
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// truncate keeps a column from swallowing the terminal. The full value is
// always in the json output.
func truncate(s string) string {
	const maxWidth = 48
	if len(s) <= maxWidth {
		return s
	}
	return s[:maxWidth-1] + "…"
}
