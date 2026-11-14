package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	for _, want := range []string{"swapsafe " + version, "commit: " + commit, "built:  " + date} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("version output missing %q; got:\n%s", want, out.String())
		}
	}
}
