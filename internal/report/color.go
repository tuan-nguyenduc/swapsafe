package report

import (
	"io"
	"os"
	"strings"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// ANSI attributes. Severity is the one thing a reader scans for, so it gets
// the colour; everything else stays out of the way.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[1;31m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
)

// palette writes colour only where it will be read by a person.
type palette struct{ on bool }

// newPalette turns colour on for a terminal, and off for everything else: a
// pipe, a file, a CI log, or a user who set NO_COLOR.
func newPalette(w io.Writer) palette {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return palette{}
	}
	f, ok := w.(*os.File)
	if !ok {
		return palette{}
	}
	info, err := f.Stat()
	return palette{on: err == nil && info.Mode()&os.ModeCharDevice != 0}
}

// paint wraps s in an attribute.
//
// Colour is applied after the table has been laid out, never inside a cell:
// tabwriter counts the width of escape sequences, even bracketed ones, so a
// coloured cell would push its column out by the length of its own colour
// codes.
func (p palette) paint(attr, s string) string {
	if !p.on || s == "" {
		return s
	}
	return attr + s + ansiReset
}

// colorizeTable paints a finished table: the header in bold, the severity at
// the head of each row, and the absent marker dimmed wherever it appears.
// Every replacement keeps the visible width it had, so the columns stay put.
func (p palette) colorizeTable(table string, tokens []string) string {
	if !p.on {
		return table
	}

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = p.bold(line)
			continue
		}
		for _, token := range tokens {
			rest, found := strings.CutPrefix(line, token)
			if found {
				line = p.token(token) + rest
				break
			}
		}
		lines[i] = strings.ReplaceAll(line, api.ValueAbsent, p.dim(api.ValueAbsent))
	}
	return strings.Join(lines, "\n") + "\n"
}

// token colours a word that leads a row: a severity, or the marker for a
// route with no counterpart.
func (p palette) token(s string) string {
	if s == "MISSING" {
		return p.red(s)
	}
	return p.severity(api.Severity(s))
}

func (p palette) bold(s string) string  { return p.paint(ansiBold, s) }
func (p palette) dim(s string) string   { return p.paint(ansiDim, s) }
func (p palette) green(s string) string { return p.paint(ansiGreen, s) }
func (p palette) red(s string) string   { return p.paint(ansiRed, s) }

// severity colours a severity by how much it should interrupt the reader.
func (p palette) severity(s api.Severity) string {
	switch s {
	case api.SeverityCritical:
		return p.paint(ansiRed, string(s))
	case api.SeverityMajor:
		return p.paint(ansiYellow, string(s))
	case api.SeverityMinor:
		return p.paint(ansiCyan, string(s))
	default:
		return p.paint(ansiDim, string(s))
	}
}

// colorizeScanTable paints the scan table, where MISSING sits in the middle
// of a row rather than at its head.
func (p palette) colorizeScanTable(table string) string {
	if !p.on {
		return table
	}

	lines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = p.bold(line)
			continue
		}
		lines[i] = strings.Replace(line, "MISSING", p.red("MISSING"), 1)
	}
	return strings.Join(lines, "\n") + "\n"
}
