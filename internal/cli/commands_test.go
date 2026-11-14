package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execute runs the command tree with args and returns its output and error.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(t.Context())
	return out.String(), err
}

func manifestFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifests.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanSendsNoRequests(t *testing.T) {
	t.Parallel()

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	defer srv.Close()

	path := manifestFile(t)
	out, err := execute(t, "scan", "--ingress-file", path, "--httproute-file", path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if hits != 0 {
		t.Errorf("scan sent %d requests; it must send none", hits)
	}
	if !strings.Contains(out, "shop/web/shop.example.com/") {
		t.Errorf("scan did not list the route:\n%s", out)
	}
	if !strings.Contains(out, "baseline") {
		t.Errorf("scan did not name the rules that would probe:\n%s", out)
	}
}

func TestScanNeedsInput(t *testing.T) {
	t.Parallel()

	if _, err := execute(t, "scan"); err == nil {
		t.Fatal("scan ran with no input files")
	}
}

func TestRunExitsTwoOnFindings(t *testing.T) {
	t.Parallel()

	baseline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://shop.example.com"+r.URL.Path)
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer baseline.Close()
	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("the real page"))
	}))
	defer candidate.Close()

	path := manifestFile(t)
	out, err := execute(t, "run",
		"--baseline", baseline.URL, "--candidate", candidate.URL,
		"--ingress-file", path, "--httproute-file", path,
		"--rate", "500")

	if err == nil {
		t.Fatalf("a critical difference did not fail the run:\n%s", out)
	}

	var coded Coder
	if !errors.As(err, &coded) {
		t.Fatalf("error does not carry an exit code: %v", err)
	}
	if coded.Code() != ExitFindings {
		t.Errorf("exit code = %d, want %d", coded.Code(), ExitFindings)
	}
	if !strings.Contains(out, "critical") {
		t.Errorf("the report was not printed before failing:\n%s", out)
	}
}

// Raising the threshold has to change the exit code, not the findings.
func TestRunFailOnThreshold(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("same"))
	}))
	defer srv.Close()

	path := manifestFile(t)
	args := []string{"run", "--baseline", srv.URL, "--candidate", srv.URL,
		"--ingress-file", path, "--httproute-file", path, "--rate", "500"}

	// Both endpoints are the same server, so only the info-level "annotation
	// not verified" findings remain.
	if out, err := execute(t, args...); err != nil {
		t.Fatalf("identical endpoints failed the run:\n%s\n%v", out, err)
	}
	if _, err := execute(t, append(args, "--fail-on", "info")...); err == nil {
		t.Error("--fail-on info did not catch the unverified annotations")
	}
}

func TestRunValidatesFlags(t *testing.T) {
	t.Parallel()

	path := manifestFile(t)
	base := []string{"run", "--baseline", "http://a", "--candidate", "http://b",
		"--ingress-file", path, "--httproute-file", path}

	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown severity", args: append(base, "--fail-on", "fatal")},
		{name: "unknown output", args: append(base, "--output", "yaml")},
		{name: "no endpoints", args: []string{"run", "--ingress-file", path}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := execute(t, tt.args...); err == nil {
				t.Error("the command accepted an invalid flag")
			}
		})
	}
}

func TestRunJSONOutput(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("same"))
	}))
	defer srv.Close()

	path := manifestFile(t)
	out, err := execute(t, "run", "--baseline", srv.URL, "--candidate", srv.URL,
		"--ingress-file", path, "--httproute-file", path, "--rate", "500",
		"--output", "json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, `"apiVersion": "swapsafe.dev/v1alpha1"`) {
		t.Errorf("json output is missing its schema:\n%s", out)
	}
	if !strings.Contains(out, `"observations"`) {
		t.Error("json output has no observations, so it cannot be re-rendered")
	}
}
