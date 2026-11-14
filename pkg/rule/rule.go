// Package rule is the extension point for annotation knowledge. Each
// ingress-nginx annotation becomes one Rule: it says when it applies, what to
// probe, and how to judge a difference.
package rule

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// Rule is the contract every annotation implements (PLAN section 5.2).
type Rule interface {
	// ID is stable and namespaced, e.g. "nginx.rewrite-target".
	ID() string
	// Annotations lists the annotation keys that activate this rule. It is
	// what `swapsafe rules` prints and what marks an annotation as verified.
	Annotations() []string
	// Applies reports whether this rule has anything to say about the route.
	Applies(r api.RouteSpec) bool
	// Generate returns the requests to send to both endpoints.
	Generate(r api.RouteSpec) []api.TestCase
	// Compare judges a pair of responses. Returning nil means "nothing beyond
	// what the generic Assertion comparison already covers".
	Compare(tc api.TestCase, base, cand api.Observation) []api.Finding
	// Hint suggests the Gateway API configuration that would close the gap.
	Hint(f api.Finding) string
}

// The registry is the one global in swapsafe, by design (PLAN section 6.1):
// rules register themselves from init() so adding a rule is adding a file.
var (
	mu         sync.RWMutex
	registered = map[string]Rule{}
)

// Register adds a rule. It is meant to be called from init() only.
//
// It panics on an empty or duplicate ID. That is a programming error in the
// binary itself, caught before main runs, and no caller inside init() could do
// anything useful with an error.
func Register(r Rule) {
	mu.Lock()
	defer mu.Unlock()

	id := r.ID()
	if id == "" {
		panic("rule: Register called with an empty ID")
	}
	if _, dup := registered[id]; dup {
		panic(fmt.Sprintf("rule: %q registered twice", id))
	}
	registered[id] = r
}

// All returns every registered rule, sorted by ID so output is deterministic.
func All() []Rule {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]Rule, 0, len(registered))
	for _, r := range registered {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// For returns the rules that apply to a route, sorted by ID.
func For(route api.RouteSpec) []Rule {
	var out []Rule
	for _, r := range All() {
		if r.Applies(route) {
			out = append(out, r)
		}
	}
	return out
}

// TestCaseID is the stable hash of (route, rule, variant) from PLAN section
// 5.1. The same input always produces the same ID, so reports from different
// runs line up and golden files stay stable.
func TestCaseID(routeID, ruleID, variant string) string {
	sum := sha256.Sum256([]byte(routeID + "\x00" + ruleID + "\x00" + variant))
	return hex.EncodeToString(sum[:8])
}
