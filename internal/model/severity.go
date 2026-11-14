package model

import "fmt"

// Severity ranks how much a difference matters. See PLAN section 5.4 for which
// kind of difference maps to which level.
type Severity string

// The severity levels, most severe first.
const (
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
	SeverityInfo     Severity = "info"
)

// rank orders the levels; a higher number is more severe. Unknown and empty
// severities rank 0, below info, so they never trip a --fail-on threshold.
var rank = map[Severity]int{
	SeverityInfo:     1,
	SeverityMinor:    2,
	SeverityMajor:    3,
	SeverityCritical: 4,
}

// ParseSeverity converts the value of a --fail-on flag or a config override.
func ParseSeverity(s string) (Severity, error) {
	sev := Severity(s)
	if _, ok := rank[sev]; !ok {
		return "", fmt.Errorf("unknown severity %q: want critical, major, minor or info", s)
	}
	return sev, nil
}

// AtLeast reports whether s is as severe as threshold. It is what --fail-on
// compares against, so an unknown or empty severity never fails a run.
func (s Severity) AtLeast(threshold Severity) bool {
	return rank[s] >= rank[threshold] && rank[s] > 0
}

// String implements fmt.Stringer.
func (s Severity) String() string { return string(s) }
