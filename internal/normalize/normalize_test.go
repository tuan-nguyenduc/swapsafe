package normalize

import (
	"net/http"
	"reflect"
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

func request() api.RequestSpec {
	return api.RequestSpec{Scheme: "https", Host: "shop.example.com", Path: "/checkout"}
}

func TestHeadersDropsNoise(t *testing.T) {
	t.Parallel()

	in := http.Header{
		"Date":                          {"Sun, 15 Nov 2026 09:00:00 GMT"},
		"Server":                        {"nginx/1.25.3"},
		"Via":                           {"1.1 google"},
		"X-Request-Id":                  {"6b1f"},
		"Traceparent":                   {"00-abc-def-01"},
		"Content-Length":                {"1234"},
		"X-Envoy-Upstream-Service-Time": {"3"},
		"Content-Type":                  {"text/html"},
	}

	got := Headers(in, request(), Options{})
	want := http.Header{"Content-Type": {"text/html"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Headers() = %v, want %v", got, want)
	}
}

func TestHeadersIgnoreList(t *testing.T) {
	t.Parallel()

	in := http.Header{"X-Cache": {"HIT"}, "Age": {"12"}, "Content-Type": {"text/html"}}

	// The config names headers however the user typed them.
	got := Headers(in, request(), Options{IgnoreHeaders: []string{"x-cache", "AGE"}})
	want := http.Header{"Content-Type": {"text/html"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Headers() = %v, want %v", got, want)
	}
}

func TestHeadersMaskSecrets(t *testing.T) {
	t.Parallel()

	in := http.Header{"Www-Authenticate": {"Basic realm=\"shop\""}}
	got := Headers(in, request(), Options{})

	if v := got.Get("Www-Authenticate"); v != Redacted {
		t.Errorf("Www-Authenticate = %q, want %q: its presence is behaviour, its value is not ours to print", v, Redacted)
	}
}

func TestHeadersSortAndDeduplicateValues(t *testing.T) {
	t.Parallel()

	in := http.Header{"Vary": {"Accept-Encoding", "Accept", "Accept-Encoding"}}
	got := Headers(in, request(), Options{})

	want := []string{"Accept", "Accept-Encoding"}
	if !reflect.DeepEqual(got["Vary"], want) {
		t.Errorf("Vary = %v, want %v: order alone must never be a difference", got["Vary"], want)
	}
}

func TestHeadersEmpty(t *testing.T) {
	t.Parallel()

	if got := Headers(nil, request(), Options{}); got != nil {
		t.Errorf("Headers(nil) = %v, want nil", got)
	}
	// A response carrying only noise normalizes to nothing, not to an empty map.
	if got := Headers(http.Header{"Date": {"now"}}, request(), Options{}); got != nil {
		t.Errorf("Headers() = %v, want nil", got)
	}
}

func TestLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty stays empty"},
		{
			name: "relative is resolved against the request",
			raw:  "/login",
			want: "https://shop.example.com/login",
		},
		{
			name: "relative without a slash resolves against the request path",
			raw:  "login",
			want: "https://shop.example.com/login",
		},
		{
			name: "default https port is dropped",
			raw:  "https://shop.example.com:443/login",
			want: "https://shop.example.com/login",
		},
		{
			name: "default http port is dropped",
			raw:  "http://shop.example.com:80/login",
			want: "http://shop.example.com/login",
		},
		{
			name: "a real port is kept",
			raw:  "https://shop.example.com:8443/login",
			want: "https://shop.example.com:8443/login",
		},
		{
			name: "host and scheme are lowercased",
			raw:  "HTTPS://SHOP.EXAMPLE.COM/login",
			want: "https://shop.example.com/login",
		},
		{
			name: "percent encoding is canonicalized",
			raw:  "https://shop.example.com/caf%C3%A9",
			want: "https://shop.example.com/caf%C3%A9",
		},
		{
			name: "query is kept",
			raw:  "https://shop.example.com/login?next=%2Fcheckout",
			want: "https://shop.example.com/login?next=%2Fcheckout",
		},
		{
			name: "another host is left where it points",
			raw:  "https://auth.example.com/sso",
			want: "https://auth.example.com/sso",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Location(tt.raw, request()); got != tt.want {
				t.Errorf("Location(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// The whole point of normalizing Location: nginx writes an absolute URL with
// the port, the Gateway writes a relative path. Same behaviour.
func TestLocationMakesTwoProxiesAgree(t *testing.T) {
	t.Parallel()

	nginx := Location("https://shop.example.com:443/login", request())
	gateway := Location("/login", request())
	if nginx != gateway {
		t.Errorf("same redirect compared unequal: %q vs %q", nginx, gateway)
	}
}

func TestSetCookie(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "value is redacted, volatile attributes dropped",
			raw:  "session=abc123; Path=/; Expires=Wed, 21 Oct 2026 07:28:00 GMT; Max-Age=3600; Secure; HttpOnly",
			want: "session=" + Redacted + "; Path=/; Secure; HttpOnly",
		},
		{
			name: "domain is lowercased",
			raw:  "id=x; Domain=Shop.Example.COM",
			want: "id=" + Redacted + "; Domain=shop.example.com",
		},
		{
			name: "samesite is kept",
			raw:  "id=x; Path=/; SameSite=Lax",
			want: "id=" + Redacted + "; Path=/; SameSite=Lax",
		},
		{
			name: "bare cookie",
			raw:  "id=x",
			want: "id=" + Redacted,
		},
		{
			name: "unparsable is not echoed back",
			raw:  "this is not a cookie at all",
			want: "unparsable set-cookie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SetCookie(tt.raw); got != tt.want {
				t.Errorf("SetCookie(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// Two sessions differ in their cookie values on every single request. If that
// leaked through, every route with a session cookie would be a false positive.
func TestSetCookieHidesTheValue(t *testing.T) {
	t.Parallel()

	first := SetCookie("session=aaaaaaaa; Path=/; Secure")
	second := SetCookie("session=bbbbbbbb; Path=/; Secure")
	if first != second {
		t.Errorf("two sessions compared unequal: %q vs %q", first, second)
	}
}

func TestObservationNormalizesHeaders(t *testing.T) {
	t.Parallel()

	obs := api.Observation{
		Endpoint: api.EndpointBaseline,
		Status:   http.StatusMovedPermanently,
		Headers:  http.Header{"Date": {"now"}, "Location": {"/login"}},
	}

	got := Observation(obs, request(), Options{})
	if got.Status != http.StatusMovedPermanently {
		t.Error("status must pass through untouched")
	}
	if got.Headers.Get("Date") != "" {
		t.Error("Date survived normalization")
	}
	if want := "https://shop.example.com/login"; got.Headers.Get("Location") != want {
		t.Errorf("Location = %q, want %q", got.Headers.Get("Location"), want)
	}
}
