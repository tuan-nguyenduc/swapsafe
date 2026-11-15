// Package prober sends each test case to both endpoints and records what came
// back. It is the only package that touches the network, and the only one that
// can put load on a production ingress — so its defaults are conservative on
// purpose (PLAN section 5.5).
package prober

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// Defaults from PLAN section 5.5. They are low enough to point at production
// without being noticed.
const (
	DefaultConcurrency   = 8
	DefaultRatePerSecond = 20
	DefaultTimeout       = 10 * time.Second
	// MaxBodySample is how much of a response body is kept for the report.
	MaxBodySample = 4 << 10
)

// safeMethods are the only methods sent unless AllowUnsafeMethods is set.
// Everything else can change state on a live system.
var safeMethods = []string{http.MethodGet, http.MethodHead, http.MethodOptions}

// Options configures a Prober. The zero value of each numeric field means
// "use the default".
type Options struct {
	// Baseline and Candidate are the addresses to connect to:
	// 203.0.113.10, gateway.internal:8080, or a full URL.
	//
	// Only the host and the port are used. The scheme is ignored, because the
	// scheme of each request comes from the rule that asked for it — probing
	// ssl-redirect means sending plain HTTP to the very endpoint you wrote
	// https in front of. Without a port, http goes to 80 and https to 443; a
	// port here overrides both.
	Baseline  string
	Candidate string

	Concurrency        int
	RatePerSecond      float64
	Timeout            time.Duration
	InsecureSkipVerify bool
	AllowUnsafeMethods bool
	// IncludeBodies keeps up to MaxBodySample bytes of each response body.
	// Off by default: a body can hold anything, and results.json is a file
	// people paste into tickets (PLAN section 6.3).
	IncludeBodies bool
}

// Prober probes two endpoints with the same requests.
type Prober struct {
	opts      Options
	endpoints []*endpoint
}

type endpoint struct {
	name   string
	host   string
	port   string // empty means "whatever the test case's scheme implies"
	client *http.Client
	// tick paces this endpoint. Each endpoint gets its own, so the rate limit
	// is per endpoint as PLAN section 5.5 specifies.
	tick *time.Ticker
}

// New validates the options and builds one HTTP client per endpoint.
func New(opts Options) (*Prober, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = DefaultConcurrency
	}
	if opts.RatePerSecond <= 0 {
		opts.RatePerSecond = DefaultRatePerSecond
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}

	p := &Prober{opts: opts}
	for _, e := range []struct{ name, raw string }{
		{api.EndpointBaseline, opts.Baseline},
		{api.EndpointCandidate, opts.Candidate},
	} {
		ep, err := newEndpoint(e.name, e.raw, opts)
		if err != nil {
			return nil, err
		}
		p.endpoints = append(p.endpoints, ep)
	}
	return p, nil
}

// Close releases the pacing tickers.
func (p *Prober) Close() {
	for _, e := range p.endpoints {
		e.tick.Stop()
		e.client.CloseIdleConnections()
	}
}

// Run probes every test case against both endpoints and returns the
// observations keyed by test case ID, baseline first.
func (p *Prober) Run(ctx context.Context, cases []api.TestCase) (map[string][]api.Observation, error) {
	if err := p.checkMethods(cases); err != nil {
		return nil, err
	}

	out := make(map[string][]api.Observation, len(cases))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.opts.Concurrency)

	for _, tc := range cases {
		for _, ep := range p.endpoints {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Wait()
				return nil, ctx.Err()
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				obs := ep.probe(ctx, tc, p.opts)

				mu.Lock()
				defer mu.Unlock()
				out[tc.ID] = append(out[tc.ID], obs)
			}()
		}
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Goroutines finish in whatever order the network allows; the report must
	// not depend on it.
	for id := range out {
		slices.SortFunc(out[id], func(a, b api.Observation) int {
			return strings.Compare(a.Endpoint, b.Endpoint)
		})
	}
	return out, nil
}

// checkMethods refuses anything that could change state unless the user asked
// for it explicitly.
func (p *Prober) checkMethods(cases []api.TestCase) error {
	var unsafe []string
	for _, tc := range cases {
		if !slices.Contains(safeMethods, tc.Request.Method) && !slices.Contains(unsafe, tc.Request.Method) {
			unsafe = append(unsafe, tc.Request.Method)
		}
	}
	if len(unsafe) == 0 {
		return nil
	}
	slices.Sort(unsafe)
	if !p.opts.AllowUnsafeMethods {
		return fmt.Errorf("refusing to send %s: these can change state on a live system; "+
			"pass --allow-unsafe-methods to send them anyway", strings.Join(unsafe, ", "))
	}
	slog.Warn("sending methods that can change state on a live system",
		"methods", strings.Join(unsafe, ", "))
	return nil
}

func newEndpoint(name, raw string, opts Options) (*endpoint, error) {
	if raw == "" {
		return nil, fmt.Errorf("%s endpoint is empty: pass --%s", name, name)
	}
	// A bare host or host:port is the natural thing to type, and url.Parse
	// would read "203.0.113.10:8080" as a scheme.
	parseable := raw
	if !strings.Contains(parseable, "://") {
		parseable = "http://" + parseable
	}

	u, err := url.Parse(parseable)
	if err != nil {
		return nil, fmt.Errorf("%s endpoint %q: %w", name, raw, err)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%s endpoint %q has no host: write an address, "+
			"for example 203.0.113.10 or https://gateway.internal:8443", name, raw)
	}

	e := &endpoint{
		name: name,
		host: u.Hostname(),
		port: u.Port(),
		tick: time.NewTicker(time.Duration(float64(time.Second) / opts.RatePerSecond)),
	}

	dialer := &net.Dialer{Timeout: opts.Timeout, KeepAlive: 30 * time.Second}
	e.client = &http.Client{
		// The request URL carries the route's hostname, so the Host header,
		// the SNI name and certificate verification are all the route's — but
		// the connection goes to this endpoint. That is what lets swapsafe
		// test a Gateway before DNS points at it.
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, e.dialAddr(addr))
			},
			TLSClientConfig: &tls.Config{
				// #nosec G402 -- off by default; only the user's explicit flag turns it on
				InsecureSkipVerify: opts.InsecureSkipVerify,
				MinVersion:         tls.VersionTLS12,
			},
			ForceAttemptHTTP2:   true,
			MaxIdleConnsPerHost: opts.Concurrency,
		},
		// A redirect is the behaviour under test, not something to follow.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return e, nil
}

// dialAddr swaps the route's host for this endpoint's, keeping the port the
// scheme implied unless the endpoint pinned one.
func (e *endpoint) dialAddr(addr string) string {
	port := e.port
	if port == "" {
		if _, p, err := net.SplitHostPort(addr); err == nil {
			port = p
		}
	}
	return net.JoinHostPort(e.host, port)
}

// probe sends one request and records the response. A network failure is
// retried once: a single dropped connection should not read as a behaviour
// difference (PLAN section 5.5).
func (e *endpoint) probe(ctx context.Context, tc api.TestCase, opts Options) api.Observation {
	obs, err := e.attempt(ctx, tc, opts)
	if err != nil && ctx.Err() == nil {
		obs, err = e.attempt(ctx, tc, opts)
	}
	if err != nil {
		obs.Err = err.Error()
	}
	return obs
}

func (e *endpoint) attempt(ctx context.Context, tc api.TestCase, opts Options) (api.Observation, error) {
	obs := api.Observation{Endpoint: e.name}

	if err := e.wait(ctx); err != nil {
		return obs, err
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	req, err := e.request(ctx, tc)
	if err != nil {
		return obs, err
	}

	start := time.Now()
	resp, err := e.client.Do(req)
	obs.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		return obs, redactURL(err)
	}
	defer func() { _ = resp.Body.Close() }()

	obs.Status = resp.StatusCode
	obs.Headers = resp.Header

	sum := sha256.New()
	sample := &strings.Builder{}
	var sink io.Writer = sum
	if opts.IncludeBodies {
		sink = io.MultiWriter(sum, &capWriter{b: sample, n: MaxBodySample})
	}
	if _, err := io.Copy(sink, resp.Body); err != nil {
		return obs, fmt.Errorf("reading body: %w", err)
	}
	obs.BodySHA256 = hex.EncodeToString(sum.Sum(nil))
	if opts.IncludeBodies {
		obs.BodySample = []byte(sample.String())
	}
	return obs, nil
}

// wait blocks until this endpoint's rate limiter lets the request through.
func (e *endpoint) wait(ctx context.Context) error {
	select {
	case <-e.tick.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *endpoint) request(ctx context.Context, tc api.TestCase) (*http.Request, error) {
	target := url.URL{
		Scheme:   tc.Request.Scheme,
		Host:     tc.Request.Host,
		Path:     tc.Request.Path,
		RawQuery: tc.Request.Query,
	}
	var body io.Reader
	if len(tc.Request.Body) > 0 {
		body = strings.NewReader(string(tc.Request.Body))
	}

	req, err := http.NewRequestWithContext(ctx, tc.Request.Method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	for k, values := range tc.Request.Headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	return req, nil
}

// redactURL strips the request URL from transport errors. It repeats the host
// swapsafe was told to probe, which may carry a userinfo component.
func redactURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("%s: %w", uerr.Op, uerr.Err)
	}
	return err
}

// capWriter keeps the first n bytes written to it and silently drops the rest,
// so a huge response cannot blow up memory just to show a sample.
type capWriter struct {
	b *strings.Builder
	n int
}

func (c *capWriter) Write(p []byte) (int, error) {
	if room := c.n - c.b.Len(); room > 0 {
		c.b.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
