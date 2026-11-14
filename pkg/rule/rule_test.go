package rule

import (
	"testing"

	"github.com/tuan-nguyenduc/swapsafe/internal/model"
)

// fakeRule is a minimal Rule for exercising the registry.
type fakeRule struct {
	id          string
	annotations []string
	applies     bool
}

func (f fakeRule) ID() string                                { return f.id }
func (f fakeRule) Annotations() []string                     { return f.annotations }
func (f fakeRule) Applies(model.RouteSpec) bool              { return f.applies }
func (f fakeRule) Generate(model.RouteSpec) []model.TestCase { return nil }
func (f fakeRule) Hint(model.Finding) string                 { return "" }
func (f fakeRule) Compare(model.TestCase, model.Observation, model.Observation) []model.Finding {
	return nil
}

// withRegistry swaps in an empty registry for the duration of a test. These
// tests mutate package state, so none of them call t.Parallel.
func withRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	saved := registered
	registered = map[string]Rule{}
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		registered = saved
		mu.Unlock()
	})
}

func TestRegisterAndAllAreSorted(t *testing.T) {
	withRegistry(t)

	Register(fakeRule{id: "nginx.zzz"})
	Register(fakeRule{id: "nginx.aaa"})
	Register(fakeRule{id: "nginx.mmm"})

	got := All()
	want := []string{"nginx.aaa", "nginx.mmm", "nginx.zzz"}
	if len(got) != len(want) {
		t.Fatalf("got %d rules, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID() != id {
			t.Errorf("rule %d = %q, want %q", i, got[i].ID(), id)
		}
	}
}

func TestRegisterRejectsBadInput(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
	}{
		{name: "empty ID", setup: func() { Register(fakeRule{}) }},
		{
			name: "duplicate ID",
			setup: func() {
				Register(fakeRule{id: "nginx.dup"})
				Register(fakeRule{id: "nginx.dup"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withRegistry(t)
			defer func() {
				if recover() == nil {
					t.Error("Register did not panic")
				}
			}()
			tt.setup()
		})
	}
}

func TestFor(t *testing.T) {
	withRegistry(t)

	Register(fakeRule{id: "nginx.yes", applies: true})
	Register(fakeRule{id: "nginx.no"})

	got := For(model.RouteSpec{ID: "shop/web/x/"})
	if len(got) != 1 || got[0].ID() != "nginx.yes" {
		t.Fatalf("For returned %v, want only nginx.yes", ids(got))
	}
}

func TestTestCaseIDIsStableAndDistinct(t *testing.T) {
	t.Parallel()

	const route, ruleID = "shop/web/shop.example.com/", "nginx.ssl-redirect"

	first := TestCaseID(route, ruleID, "http /")
	if first != TestCaseID(route, ruleID, "http /") {
		t.Error("the same input produced two different IDs")
	}

	// Every part of the triple must change the hash, and the separator must
	// not be forgeable by shifting text between the parts.
	others := []string{
		TestCaseID(route, ruleID, "https /"),
		TestCaseID(route, "nginx.rewrite-target", "http /"),
		TestCaseID("shop/web/shop.example.com/checkout", ruleID, "http /"),
		TestCaseID(route+ruleID, "", "http /"),
	}
	for i, other := range others {
		if other == first {
			t.Errorf("variant %d collided with the original ID", i)
		}
	}
}

func ids(rules []Rule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.ID())
	}
	return out
}
