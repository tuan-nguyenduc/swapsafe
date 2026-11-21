package prober

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// testCase builds a GET for shop.example.com, the host the prober must put in
// the Host header even though it dials the test server's address.
func testCase(id, path string) api.TestCase {
	return api.TestCase{
		ID:      id,
		RouteID: "shop/web/shop.example.com/",
		RuleID:  "baseline",
		Request: api.RequestSpec{
			Method: http.MethodGet,
			Scheme: "http",
			Host:   "shop.example.com",
			Path:   path,
		},
	}
}

// fastOptions keeps the rate limiter out of the way of the tests.
func fastOptions(baseline, candidate string) Options {
	return Options{
		Baseline:      baseline,
		Candidate:     candidate,
		RatePerSecond: 1000,
		Timeout:       5 * time.Second,
	}
}

func newProber(t *testing.T, opts Options) *Prober {
	t.Helper()
	p, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestRunProbesBothEndpoints(t *testing.T) {
	t.Parallel()

	var baselineHost, candidateHost atomic.Value
	baseline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baselineHost.Store(r.Host)
		w.Header().Set("Location", "https://shop.example.com/")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer baseline.Close()

	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		candidateHost.Store(r.Host)
		_, _ = w.Write([]byte("hello"))
	}))
	defer candidate.Close()

	p := newProber(t, fastOptions(baseline.URL, candidate.URL))

	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	obs := got["a"]
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want 2: %+v", len(obs), obs)
	}
	if obs[0].Endpoint != api.EndpointBaseline || obs[1].Endpoint != api.EndpointCandidate {
		t.Errorf("observations are not in a stable order: %q, %q", obs[0].Endpoint, obs[1].Endpoint)
	}
	if obs[0].Status != http.StatusMovedPermanently {
		t.Errorf("baseline status = %d, want 301 (the redirect must not be followed)", obs[0].Status)
	}
	if obs[0].Headers.Get("Location") == "" {
		t.Error("baseline Location header was lost")
	}
	if obs[1].Status != http.StatusOK {
		t.Errorf("candidate status = %d, want 200", obs[1].Status)
	}
	if obs[0].BodySHA256 == obs[1].BodySHA256 {
		t.Error("different bodies hashed the same")
	}
	if obs[0].Err != "" || obs[1].Err != "" {
		t.Errorf("unexpected errors: %q, %q", obs[0].Err, obs[1].Err)
	}

	// The whole point: dial the endpoint, but present the route's host.
	if h, _ := baselineHost.Load().(string); h != "shop.example.com" {
		t.Errorf("baseline saw Host %q, want shop.example.com", h)
	}
	if h, _ := candidateHost.Load().(string); h != "shop.example.com" {
		t.Errorf("candidate saw Host %q, want shop.example.com", h)
	}
}

func TestBodiesAreNotKeptByDefault(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret-ish body"))
	}))
	defer srv.Close()

	p := newProber(t, fastOptions(srv.URL, srv.URL))
	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, obs := range got["a"] {
		if len(obs.BodySample) != 0 {
			t.Errorf("body kept without IncludeBodies: %q", obs.BodySample)
		}
		if obs.BodySHA256 == "" {
			t.Error("body hash is missing; the differ needs it")
		}
	}
}

func TestIncludeBodiesIsCapped(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("x", MaxBodySample*3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()

	opts := fastOptions(srv.URL, srv.URL)
	opts.IncludeBodies = true
	p := newProber(t, opts)

	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	obs := got["a"][0]
	if len(obs.BodySample) != MaxBodySample {
		t.Errorf("sample is %d bytes, want %d", len(obs.BodySample), MaxBodySample)
	}
	// The hash must still cover the whole body, not just the sample.
	if obs.BodySHA256 == "" {
		t.Fatal("body hash is missing")
	}
	short := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(big[:MaxBodySample]))
	}))
	defer short.Close()

	p2 := newProber(t, func() Options { o := fastOptions(short.URL, short.URL); o.IncludeBodies = true; return o }())
	got2, err := p2.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got2["a"][0].BodySHA256 == obs.BodySHA256 {
		t.Error("truncated body hashed the same as the full one")
	}
}

func TestUnsafeMethodsAreRefused(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	del := testCase("a", "/")
	del.Request.Method = http.MethodDelete

	p := newProber(t, fastOptions(srv.URL, srv.URL))
	_, err := p.Run(t.Context(), []api.TestCase{del})
	if err == nil {
		t.Fatal("DELETE was sent without --allow-unsafe-methods")
	}
	if !strings.Contains(err.Error(), "allow-unsafe-methods") {
		t.Errorf("error does not say how to proceed: %v", err)
	}

	opts := fastOptions(srv.URL, srv.URL)
	opts.AllowUnsafeMethods = true
	allowed := newProber(t, opts)
	if _, err := allowed.Run(t.Context(), []api.TestCase{del}); err != nil {
		t.Errorf("DELETE refused even with AllowUnsafeMethods: %v", err)
	}
}

func TestSafeMethodsPass(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	p := newProber(t, fastOptions(srv.URL, srv.URL))

	for _, method := range safeMethods {
		tc := testCase("a", "/")
		tc.Request.Method = method
		if _, err := p.Run(t.Context(), []api.TestCase{tc}); err != nil {
			t.Errorf("%s was refused: %v", method, err)
		}
	}
}

func TestNetworkErrorIsRecordedNotFatal(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	// Nothing listens on the candidate port.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	p := newProber(t, fastOptions(srv.URL, deadURL))
	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run returned an error for an unreachable endpoint: %v", err)
	}
	if got["a"][0].Err != "" {
		t.Errorf("baseline reported an error: %q", got["a"][0].Err)
	}
	if got["a"][1].Err == "" {
		t.Error("an unreachable candidate must be recorded as an error on the observation")
	}
}

func TestRateLimitIsPerEndpoint(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	opts := fastOptions(srv.URL, srv.URL)
	opts.RatePerSecond = 20 // 50ms apart
	p := newProber(t, opts)

	cases := []api.TestCase{testCase("a", "/a"), testCase("b", "/b"), testCase("c", "/c")}

	start := time.Now()
	if _, err := p.Run(t.Context(), cases); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(start)

	// Three requests per endpoint at 20/s: the first is immediate, so at
	// least two intervals must have passed.
	if want := 2 * 50 * time.Millisecond; elapsed < want {
		t.Errorf("3 requests at 20/s took %v, want at least %v", elapsed, want)
	}
	if got := hits.Load(); got != 6 {
		t.Errorf("server saw %d requests, want 6 (3 cases x 2 endpoints)", got)
	}
}

func TestRunRespectsCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	p := newProber(t, fastOptions(srv.URL, srv.URL))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := p.Run(ctx, []api.TestCase{testCase("a", "/")}); err == nil {
		t.Fatal("Run ignored a cancelled context")
	}
}

func TestNewRejectsBadEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		baseline, candidate string
	}{
		{name: "empty baseline", candidate: "http://127.0.0.1:1"},
		{name: "empty candidate", baseline: "http://127.0.0.1:1"},
		// A bare hostname is fine now; a path with no host is not.
		{name: "no host", baseline: "http://127.0.0.1:1", candidate: "/just/a/path"},
		{name: "scheme only", baseline: "http://127.0.0.1:1", candidate: "https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(Options{Baseline: tt.baseline, Candidate: tt.candidate}); err == nil {
				t.Error("New accepted an unusable endpoint")
			}
		})
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	t.Parallel()

	p := newProber(t, Options{Baseline: "http://a", Candidate: "http://b"})
	if p.opts.Concurrency != DefaultConcurrency ||
		p.opts.RatePerSecond != DefaultRatePerSecond ||
		p.opts.Timeout != DefaultTimeout {
		t.Errorf("defaults not applied: %+v", p.opts)
	}
	if p.opts.InsecureSkipVerify || p.opts.AllowUnsafeMethods || p.opts.IncludeBodies {
		t.Error("a safety flag defaults to on")
	}
}

func TestDialAddrKeepsSchemePortUnlessPinned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint endpoint
		addr     string
		want     string
	}{
		{
			name:     "scheme port is kept",
			endpoint: endpoint{host: "203.0.113.10"},
			addr:     "shop.example.com:443",
			want:     "203.0.113.10:443",
		},
		{
			name:     "endpoint port wins",
			endpoint: endpoint{host: "203.0.113.10", port: "8080"},
			addr:     "shop.example.com:80",
			want:     "203.0.113.10:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.endpoint.dialAddr(tt.addr); got != tt.want {
				t.Errorf("dialAddr(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// A bare host or host:port is the natural thing to type, and it is what
// kubectl prints.
func TestNewAcceptsBareAddresses(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	address := strings.TrimPrefix(srv.URL, "http://")

	opts := fastOptions(address, address)
	p := newProber(t, opts)

	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got["a"][0].Err != "" {
		t.Errorf("a bare host:port was not usable: %q", got["a"][0].Err)
	}
}

// The scheme on the flag is not the scheme of the request: probing
// ssl-redirect means sending plain HTTP to an endpoint written as https.
func TestEndpointSchemeIsIgnored(t *testing.T) {
	t.Parallel()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	// https in front of a plaintext test server: the port is what decides.
	opts := fastOptions("https://"+strings.TrimPrefix(srv.URL, "http://"), srv.URL)
	p := newProber(t, opts)

	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got["a"][0].Err != "" {
		t.Errorf("the endpoint scheme changed how the request was sent: %q", got["a"][0].Err)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("server saw %d requests, want 2", hits)
	}
}

// counterHandler answers with a body that changes on every request, the way
// an application that reports a request id, uptime or pod name does.
func counterHandler(hits *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"hits": %d}`, hits.Add(1))
	}
}

func TestUnstableBaselineBodyIsMarked(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	baseline := httptest.NewServer(counterHandler(&hits))
	defer baseline.Close()
	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("something else"))
	}))
	defer candidate.Close()

	p := newProber(t, fastOptions(baseline.URL, candidate.URL))
	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, o := range got["a"] {
		if !o.BodyUnstable {
			t.Errorf("%s observation was not marked unstable", o.Endpoint)
		}
	}
	if hits.Load() != 3 {
		t.Errorf("baseline saw %d requests, want 3: the original probe and two re-checks", hits.Load())
	}
}

// A real difference between two stable bodies must survive untouched.
func TestStableBodyDifferenceIsNotMarked(t *testing.T) {
	t.Parallel()

	baseline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("page a"))
	}))
	defer baseline.Close()
	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("page b"))
	}))
	defer candidate.Close()

	p := newProber(t, fastOptions(baseline.URL, candidate.URL))
	got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, o := range got["a"] {
		if o.BodyUnstable {
			t.Errorf("%s observation was marked unstable though both bodies are stable", o.Endpoint)
		}
	}
}

// The whole point of re-probing only on a difference: a correctly migrated
// cluster pays nothing for the check.
func TestMatchingBodiesCostNoExtraRequests(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	same := func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("identical"))
	}
	baseline := httptest.NewServer(http.HandlerFunc(same))
	defer baseline.Close()
	candidate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("identical"))
	}))
	defer candidate.Close()

	p := newProber(t, fastOptions(baseline.URL, candidate.URL))
	cases := []api.TestCase{testCase("a", "/a"), testCase("b", "/b"), testCase("c", "/c")}
	if _, err := p.Run(t.Context(), cases); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := hits.Load(); got != int32(len(cases)) {
		t.Errorf("baseline saw %d requests for %d cases: the stability check is not free when bodies match", got, len(cases))
	}
}

// jsonHandler answers with a stable field and a volatile one, the shape of
// every application that reports its own uptime or goroutine count.
func jsonHandler(page string, counter *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"page": %q, "num_goroutine": %d}`, page, counter.Add(1))
	}
}

// The point of comparing field by field: a volatile field is dropped and
// everything else is still compared.
func TestVolatileFieldIsIgnoredAndTheRestIsCompared(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		basePage       string
		candPage       string
		wantSameDigest bool
	}{
		{name: "only the volatile field differs", basePage: "home", candPage: "home", wantSameDigest: true},
		{name: "a real difference survives", basePage: "home", candPage: "login"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var baseCounter, candCounter atomic.Int32
			// The two counters start apart, so the bodies differ for a reason
			// that is nobody's fault — which is the whole case under test.
			candCounter.Store(1000)
			baseline := httptest.NewServer(jsonHandler(tt.basePage, &baseCounter))
			defer baseline.Close()
			candidate := httptest.NewServer(jsonHandler(tt.candPage, &candCounter))
			defer candidate.Close()

			p := newProber(t, fastOptions(baseline.URL, candidate.URL))
			got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			base, cand := got["a"][0], got["a"][1]
			if base.BodyUnstable || cand.BodyUnstable {
				t.Fatal("a JSON body was given up on instead of compared field by field")
			}
			if !slices.Contains(base.BodyIgnoredFields, "num_goroutine") {
				t.Errorf("num_goroutine was not recognized as volatile: %v", base.BodyIgnoredFields)
			}
			if slices.Contains(base.BodyIgnoredFields, "page") {
				t.Errorf("a stable field was dropped from the comparison: %v", base.BodyIgnoredFields)
			}

			same := base.BodySHA256 == cand.BodySHA256
			if same != tt.wantSameDigest {
				t.Errorf("digests equal = %v, want %v (%q against %q)", same, tt.wantSameDigest, tt.basePage, tt.candPage)
			}
		})
	}
}

// The failure the lab run reported: the same cluster, five runs, two of them
// claiming a body difference. Twenty runs here, all of which must agree.
func TestRepeatedRunsAgreeOnAVolatileBody(t *testing.T) {
	t.Parallel()

	var baseCounter, candCounter atomic.Int32
	candCounter.Store(1000)
	baseline := httptest.NewServer(jsonHandler("home", &baseCounter))
	defer baseline.Close()
	candidate := httptest.NewServer(jsonHandler("home", &candCounter))
	defer candidate.Close()

	p := newProber(t, fastOptions(baseline.URL, candidate.URL))

	for i := range 20 {
		got, err := p.Run(t.Context(), []api.TestCase{testCase("a", "/")})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		base, cand := got["a"][0], got["a"][1]
		if base.BodySHA256 != cand.BodySHA256 {
			t.Fatalf("run %d reported a body difference between two identical pages", i)
		}
	}
}
