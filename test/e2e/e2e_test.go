//go:build e2e

// Package e2e runs swapsafe against a real ingress-nginx and a real Envoy
// Gateway, both serving the same hostnames in a kind cluster.
//
// Everything else in the test suite talks to httptest handlers that answer
// exactly what the test told them to. This is the only place that finds out
// what two actual proxies disagree about — which is the whole question
// swapsafe exists to answer, and the only way to know the normalizer is
// pulling its weight.
//
//	hack/kind-up.sh && make e2e
package e2e

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tuan-nguyenduc/swapsafe/internal/cli"
	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

var update = flag.Bool("update", false, "rewrite the golden file from this run")

const (
	namespace   = "swapsafe-e2e"
	goldenFile  = "../golden/e2e-findings.json"
	forwardWait = 30 * time.Second
)

// finding is a finding reduced to what a cluster cannot make vary: latency,
// hashes and addresses are all left out.
type finding struct {
	Route    string `json:"route"`
	Request  string `json:"request"`
	Aspect   string `json:"aspect"`
	Severity string `json:"severity"`
}

func TestEndToEnd(t *testing.T) {
	baseline := portForward(t, "ingress-nginx", "svc/ingress-nginx-controller")
	candidate := portForward(t, "envoy-gateway-system", envoyService(t))

	res := run(t, "run",
		"--from-cluster", "--namespace", namespace, "--ingress-class", "nginx",
		"--baseline", baseline, "--candidate", candidate,
		"--output", "json", "--rate", "200", "--fail-on", "info")

	got := reduce(res)
	if *update {
		writeGolden(t, got)
		t.Logf("golden file rewritten with %d findings", len(got))
		return
	}

	want := readGolden(t)
	if diff := compare(want, got); diff != "" {
		t.Errorf("findings do not match %s:\n%s\n\nRe-run with -update after reviewing.", goldenFile, diff)
	}
}

// TestNoFalsePositives is the half of the suite that decides whether anyone
// keeps using swapsafe. ok.e2e.test is translated correctly, so nginx and
// Envoy must look identical through the normalizer — different Server
// headers, different request ids, different timings and all.
func TestNoFalsePositives(t *testing.T) {
	baseline := portForward(t, "ingress-nginx", "svc/ingress-nginx-controller")
	candidate := portForward(t, "envoy-gateway-system", envoyService(t))

	res := run(t, "run",
		"--from-cluster", "--namespace", namespace, "--ingress-class", "nginx",
		"--baseline", baseline, "--candidate", candidate,
		"--output", "json", "--rate", "200", "--fail-on", "info")

	for _, f := range res.Findings {
		if strings.Contains(f.RouteID, "/ok/") {
			t.Errorf("a correctly translated route produced a finding: %s %s\n  baseline:  %s\n  candidate: %s",
				f.Severity, f.Aspect, f.Baseline, f.Candidate)
		}
	}
}

// run executes the swapsafe command tree and parses its json output.
func run(t *testing.T, args ...string) api.Results {
	t.Helper()

	var out bytes.Buffer
	cmd := cli.NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	// A run that finds differences exits 2; here that is the expected case.
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		var coded cli.Coder
		if !errorAs(err, &coded) {
			t.Fatalf("swapsafe failed: %v\n%s", err, out.String())
		}
	}

	var res api.Results
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output is not the results document: %v\n%s", err, out.String())
	}
	if len(res.Routes) == 0 {
		t.Fatal("no routes were collected; is the cluster up? run hack/kind-up.sh")
	}
	return res
}

func errorAs(err error, target *cli.Coder) bool {
	coded, ok := err.(cli.Coder)
	if ok {
		*target = coded
	}
	return ok
}

// reduce drops everything a second run could legitimately change.
func reduce(res api.Results) []finding {
	requests := make(map[string]string, len(res.TestCases))
	for _, tc := range res.TestCases {
		requests[tc.ID] = tc.Request.Method + " " + tc.Request.Path
	}

	out := make([]finding, 0, len(res.Findings))
	for _, f := range res.Findings {
		request := requests[f.TestCaseID]
		if f.TestCaseID == "" {
			request = "-"
		}
		out = append(out, finding{
			Route:    shortRoute(f.RouteID),
			Request:  request,
			Aspect:   f.Aspect,
			Severity: string(f.Severity),
		})
	}
	slices.SortFunc(out, func(a, b finding) int {
		return strings.Compare(a.Route+a.Request+a.Aspect, b.Route+b.Request+b.Aspect)
	})
	return out
}

// shortRoute drops the namespace, which is the same for every fixture.
func shortRoute(id string) string { return strings.TrimPrefix(id, namespace+"/") }

func compare(want, got []finding) string {
	var sb strings.Builder
	for _, f := range got {
		if !slices.Contains(want, f) {
			fmt.Fprintf(&sb, "  unexpected: %+v\n", f)
		}
	}
	for _, f := range want {
		if !slices.Contains(got, f) {
			fmt.Fprintf(&sb, "  missing:    %+v\n", f)
		}
	}
	return sb.String()
}

func readGolden(t *testing.T) []finding {
	t.Helper()
	data, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("%v\n\nRun once with -update to create it.", err)
	}
	var want []finding
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("reading %s: %v", goldenFile, err)
	}
	return want
}

func writeGolden(t *testing.T, got []finding) {
	t.Helper()
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(goldenFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenFile, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// envoyService finds the Service the Gateway's proxy sits behind. Its name
// carries a hash, so it cannot be written down.
func envoyService(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "kubectl", "-n", "envoy-gateway-system",
		"get", "svc", "-l", "gateway.envoyproxy.io/owning-gateway-name=swapsafe",
		"-o", "name").Output()
	if err != nil {
		t.Fatalf("finding the Envoy service: %v", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		t.Fatal("no Envoy service for the swapsafe Gateway; run hack/kind-up.sh")
	}
	return name
}

// portForward opens a tunnel to a Service on a free local port and returns
// the endpoint URL. The Gateway's Service is a LoadBalancer with no address
// in kind, so forwarding is how both ends are reached.
func portForward(t *testing.T, namespace, service string) string {
	t.Helper()

	port := freePort(t)
	cmd := exec.Command("kubectl", "-n", namespace, "port-forward", service,
		fmt.Sprintf("%d:80", port))
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting port-forward to %s: %v", service, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	address := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(forwardWait)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return "http://" + address
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("port-forward to %s never came up on %s", service, address)
	return ""
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}
