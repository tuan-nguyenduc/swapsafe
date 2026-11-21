package prober

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
)

// maxComparableBody is how much of a body is held in memory to compare it
// field by field. Past that the body is only ever compared by hash.
const maxComparableBody = 1 << 20

// volatilePaths returns the JSON paths whose values differ between two
// samples of the same response. Those are the fields the application changes
// on its own — a counter, an uptime, a pod name — and comparing the two
// endpoints on them says nothing.
//
// Both bodies must be JSON; ok is false otherwise.
func volatilePaths(a, b []byte) (paths []string, ok bool) {
	va, okA := decodeJSON(a)
	vb, okB := decodeJSON(b)
	if !okA || !okB {
		return nil, false
	}
	return collectDifferences(va, vb, ""), true
}

// digestIgnoring hashes a JSON body with the given paths removed, so two
// responses that differ only in their volatile fields hash the same.
func digestIgnoring(body []byte, paths []string) (string, bool) {
	v, ok := decodeJSON(body)
	if !ok {
		return "", false
	}
	for _, p := range paths {
		v = removePath(v, p)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), true
}

func decodeJSON(body []byte) (any, bool) {
	if len(body) == 0 || len(body) > maxComparableBody {
		return nil, false
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	return v, true
}

// collectDifferences walks two decoded documents and returns the paths where
// they disagree. A path is dotted, with array indices in brackets:
// "status.uptime", "items[0].id".
func collectDifferences(a, b any, prefix string) []string {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return []string{prefix}
		}
		var out []string
		for _, key := range sortedKeys(av, bv) {
			left, inA := av[key]
			right, inB := bv[key]
			if !inA || !inB {
				out = append(out, join(prefix, key))
				continue
			}
			out = append(out, collectDifferences(left, right, join(prefix, key))...)
		}
		return out

	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return []string{prefix}
		}
		var out []string
		for i := range av {
			out = append(out, collectDifferences(av[i], bv[i], prefix+"["+strconv.Itoa(i)+"]")...)
		}
		return out

	default:
		if fmt.Sprint(a) != fmt.Sprint(b) {
			return []string{prefix}
		}
		return nil
	}
}

// removePath deletes one dotted path from a decoded document. An index inside
// a path is not followed: a volatile element makes the whole array volatile,
// which is the safe reading.
func removePath(v any, path string) any {
	if path == "" {
		return nil
	}
	head, rest := split(path)
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if rest == "" {
		delete(m, head)
		return m
	}
	if child, exists := m[head]; exists {
		m[head] = removePath(child, rest)
	}
	return m
}

func sortedKeys(a, b map[string]any) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// split cuts a dotted path into its first element and the rest, ignoring dots
// inside bracketed indices.
func split(path string) (head, rest string) {
	for i := range len(path) {
		switch path[i] {
		case '.':
			return path[:i], path[i+1:]
		case '[':
			return path[:i], ""
		}
	}
	return path, ""
}
