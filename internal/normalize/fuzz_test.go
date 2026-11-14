package normalize

import (
	"net/http"
	"strings"
	"testing"

	api "github.com/tuan-nguyenduc/swapsafe/pkg/api/v1alpha1"
)

// Normalizing has to be idempotent. The differ compares normalized values,
// and reports are re-rendered from results.json, so a function that keeps
// changing its own output would make a run disagree with itself.

func FuzzLocation(f *testing.F) {
	for _, seed := range []string{
		"", "/", "/login", "login", "//evil.example.com/",
		"https://shop.example.com:443/login",
		"http://SHOP.example.com:80/caf%C3%A9?a=1&b=%2F",
		"https://[2001:db8::1]:443/x", "https://[2001:db8::1]:8443/x",
		"mailto:someone@example.com", "://", "%%", "\x00",
	} {
		f.Add(seed)
	}

	req := api.RequestSpec{Scheme: "https", Host: "shop.example.com", Path: "/checkout"}

	f.Fuzz(func(t *testing.T, raw string) {
		once := Location(raw, req)
		if twice := Location(once, req); twice != once {
			t.Errorf("Location is not idempotent:\n  in: %q\n  1x: %q\n  2x: %q", raw, once, twice)
		}
	})
}

func FuzzSetCookie(f *testing.F) {
	for _, seed := range []string{
		"", "id=x", "id=x; Path=/", "session=abc; Path=/; Secure; HttpOnly",
		"id=x; Domain=Example.COM; SameSite=None; Secure",
		"id=x; Expires=Wed, 21 Oct 2026 07:28:00 GMT; Max-Age=0",
		"id=x; Partitioned", "not a cookie", "=", ";;;", "a=b; Path=\x00",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		once := SetCookie(raw)
		if twice := SetCookie(once); twice != once {
			t.Errorf("SetCookie is not idempotent:\n  in: %q\n  1x: %q\n  2x: %q", raw, once, twice)
		}

		// Whatever the input, the cookie value must never survive into the
		// output: it is the single most sensitive thing in a response.
		if once == "unparsable set-cookie" {
			return
		}
		c, err := http.ParseSetCookie(once)
		if err != nil {
			t.Fatalf("SetCookie produced something it cannot parse back: %q (%v)", once, err)
		}
		if c.Value != Redacted {
			t.Errorf("cookie value leaked: input %q produced %q", raw, once)
		}
		if strings.Contains(once, "Expires=") || strings.Contains(once, "Max-Age=") {
			t.Errorf("a clock-dependent attribute survived: %q", once)
		}
	})
}
