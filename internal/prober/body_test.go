package prober

import (
	"slices"
	"testing"
)

func TestVolatilePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		a, b   string
		want   []string
		wantOK bool
	}{
		{
			name:   "one field moves",
			a:      `{"page":"home","num_goroutine":9}`,
			b:      `{"page":"home","num_goroutine":11}`,
			want:   []string{"num_goroutine"},
			wantOK: true,
		},
		{
			name:   "identical documents have nothing volatile",
			a:      `{"page":"home"}`,
			b:      `{"page":"home"}`,
			wantOK: true,
		},
		{
			name:   "key order is not a difference",
			a:      `{"a":1,"b":2}`,
			b:      `{"b":2,"a":1}`,
			wantOK: true,
		},
		{
			name:   "nested fields are named by path",
			a:      `{"status":{"uptime":3,"name":"x"}}`,
			b:      `{"status":{"uptime":4,"name":"x"}}`,
			want:   []string{"status.uptime"},
			wantOK: true,
		},
		{
			name:   "a missing key counts",
			a:      `{"a":1,"b":2}`,
			b:      `{"a":1}`,
			want:   []string{"b"},
			wantOK: true,
		},
		{name: "not json", a: "page=/", b: "page=/", wantOK: false},
		{name: "empty", a: "", b: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := volatilePaths([]byte(tt.a), []byte(tt.b))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("volatilePaths = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDigestIgnoring(t *testing.T) {
	t.Parallel()

	// Two responses that differ only in the volatile field must hash the same,
	// and a difference anywhere else must survive.
	base := []byte(`{"page":"home","num_goroutine":9}`)
	sameButMoving := []byte(`{"page":"home","num_goroutine":4211}`)
	reallyDifferent := []byte(`{"page":"login","num_goroutine":9}`)

	want, ok := digestIgnoring(base, []string{"num_goroutine"})
	if !ok {
		t.Fatal("digestIgnoring refused a JSON body")
	}
	if got, _ := digestIgnoring(sameButMoving, []string{"num_goroutine"}); got != want {
		t.Error("the volatile field still changed the digest")
	}
	if got, _ := digestIgnoring(reallyDifferent, []string{"num_goroutine"}); got == want {
		t.Error("a real difference was hidden by ignoring the volatile field")
	}

	// Key order must never matter.
	a, _ := digestIgnoring([]byte(`{"a":1,"b":2}`), nil)
	b, _ := digestIgnoring([]byte(`{"b":2,"a":1}`), nil)
	if a != b {
		t.Error("two documents with the same fields in a different order hashed differently")
	}

	if _, ok := digestIgnoring([]byte("path=/"), nil); ok {
		t.Error("a non-JSON body was accepted")
	}
}

func TestRemovePathLeavesTheRestAlone(t *testing.T) {
	t.Parallel()

	full, _ := digestIgnoring([]byte(`{"keep":1,"drop":2}`), nil)
	dropped, _ := digestIgnoring([]byte(`{"keep":1,"drop":2}`), []string{"drop"})
	kept, _ := digestIgnoring([]byte(`{"keep":1}`), nil)

	if dropped != kept {
		t.Error("removing a field did not give the document without it")
	}
	if dropped == full {
		t.Error("removing a field changed nothing")
	}
}
