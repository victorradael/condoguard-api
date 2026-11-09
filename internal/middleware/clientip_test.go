package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/pkg/ratelimit"
)

// proxy is the trusted proxy of these tests (Caddy on the compose network).
var proxy = []netip.Prefix{netip.MustParsePrefix("172.28.0.0/24")}

// loginVia sends a failed login from remote with the given X-Real-IP
// values (none when realIP is empty).
func loginVia(h http.Handler, remote string, realIP ...string) int {
	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"ana@x.io","password":"guess"}`))
	req.RemoteAddr = remote
	for _, v := range realIP {
		req.Header.Add("X-Real-IP", v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func limitedLogin(trusted []netip.Prefix) http.Handler {
	return middleware.ClientIP(trusted)(
		middleware.LoginLimit(ratelimit.NewMemory(2, time.Minute), nil)(fakeLogin(nil)))
}

func TestClientIP_FromTrustedProxy_SeparatesClients(t *testing.T) {
	h := limitedLogin(proxy)
	for i := 0; i < 2; i++ {
		loginVia(h, "172.28.0.10:5000", "203.0.113.1")
	}
	if c := loginVia(h, "172.28.0.10:5000", "203.0.113.1"); c != http.StatusTooManyRequests {
		t.Fatalf("client 1 must be locked, got %d", c)
	}
	if c := loginVia(h, "172.28.0.10:5000", "203.0.113.2"); c != http.StatusUnauthorized {
		t.Errorf("another client behind the proxy must not be locked, got %d", c)
	}
	if c := loginVia(h, "172.28.0.10:5000", "2001:db8::1"); c != http.StatusUnauthorized {
		t.Errorf("an IPv6 client behind the proxy must not be locked, got %d", c)
	}
}

func TestClientIP_Security_HeaderFromUntrustedPeerIgnored(t *testing.T) {
	for name, trusted := range map[string][]netip.Prefix{"outside the list": proxy, "no list": nil} {
		t.Run(name, func(t *testing.T) {
			h := limitedLogin(trusted)
			// An attacker rotating X-Real-IP keeps hitting the same counter.
			for i, ip := range []string{"203.0.113.1", "203.0.113.2"} {
				if c := loginVia(h, "198.51.100.7:5000", ip); c != http.StatusUnauthorized {
					t.Fatalf("attempt %d: got %d", i, c)
				}
			}
			if c := loginVia(h, "198.51.100.7:5000", "203.0.113.3"); c != http.StatusTooManyRequests {
				t.Errorf("X-Real-IP from an untrusted peer must be ignored, got %d", c)
			}
		})
	}
}

func TestClientIP_InvalidHeaderFallsBackToRemoteAddr(t *testing.T) {
	for name, values := range map[string][]string{
		"missing":   nil,
		"empty":     {""},
		"garbage":   {"not-an-ip"},
		"with port": {"203.0.113.1:80"},
		"list":      {"203.0.113.1, 203.0.113.2"},
		"zone":      {"fe80::1%eth0"},
		"repeated":  {"203.0.113.1", "203.0.113.2"},
	} {
		t.Run(name, func(t *testing.T) {
			h := limitedLogin(proxy)
			// Locks the proxy's own address...
			loginVia(h, "172.28.0.10:5000")
			loginVia(h, "172.28.0.10:5000")
			// ...so an invalid header counts against it, not a client of its own.
			if c := loginVia(h, "172.28.0.10:5000", values...); c != http.StatusTooManyRequests {
				t.Errorf("X-Real-IP %q must fall back to RemoteAddr, got %d", values, c)
			}
			if c := loginVia(h, "172.28.0.10:5000", "203.0.113.9"); c != http.StatusUnauthorized {
				t.Errorf("a valid client IP must still get its own counter, got %d", c)
			}
		})
	}
}

func TestClientIP_IPv4MappedIsOneClient(t *testing.T) {
	h := limitedLogin(proxy)
	loginVia(h, "172.28.0.10:5000", "203.0.113.1")
	loginVia(h, "172.28.0.10:5000", "::ffff:203.0.113.1")
	if c := loginVia(h, "[::ffff:172.28.0.10]:5000", "203.0.113.1"); c != http.StatusTooManyRequests {
		t.Errorf("IPv4-mapped addresses must share the IPv4 counter, got %d", c)
	}
}

func TestClientIP_SignUpLimitPerClient(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) })
	h := middleware.ClientIP(proxy)(middleware.IPLimit(ratelimit.NewMemory(1, time.Minute), "slow down")(ok))
	register := func(realIP string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/register", nil)
		req.RemoteAddr = "172.28.0.10:5000"
		req.Header.Set("X-Real-IP", realIP)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if register("203.0.113.1") != http.StatusCreated || register("203.0.113.1") != http.StatusTooManyRequests {
		t.Fatal("second sign-up from the same client must be limited")
	}
	if c := register("203.0.113.2"); c != http.StatusCreated {
		t.Errorf("another client behind the proxy must be able to sign up, got %d", c)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := middleware.ParseTrustedProxies(" 172.28.0.0/24 , 10.1.2.3,fd00::/8,")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"172.28.0.0/24", "10.1.2.3/32", "fd00::/8"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Errorf("[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if got, err := middleware.ParseTrustedProxies(""); err != nil || len(got) != 0 {
		t.Errorf("empty list: %v, %v", got, err)
	}
	for _, bad := range []string{"proxy", "10.0.0.0/33", "fe80::1%eth0"} {
		if _, err := middleware.ParseTrustedProxies(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}
