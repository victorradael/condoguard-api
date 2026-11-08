package middleware_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/pkg/ratelimit"
)

// fakeLogin returns 200 for password "ok", 401 otherwise, and echoes that it
// could still read the whole body.
func fakeLogin(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"password":"ok"`) {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
}

func login(h http.Handler, ip, user, pass string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"username":"`+user+`","password":"`+pass+`"}`))
	req.RemoteAddr = ip + ":12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginLimit_BlocksAfterMaxFailures(t *testing.T) {
	h := middleware.LoginLimit(ratelimit.NewMemory(3, time.Minute))(fakeLogin(t))

	for i := 0; i < 3; i++ {
		if rec := login(h, "10.0.0.1", "ana", "bad"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}
	rec := login(h, "10.0.0.1", "ana", "ok")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 even with the right password, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("expected JSON error body, got %q", rec.Body.String())
	}

	// Other username from the same IP and same username from another IP are unaffected.
	if rec := login(h, "10.0.0.1", "bia", "ok"); rec.Code != http.StatusOK {
		t.Errorf("other user: expected 200, got %d", rec.Code)
	}
	if rec := login(h, "10.0.0.2", "ana", "ok"); rec.Code != http.StatusOK {
		t.Errorf("other IP: expected 200, got %d", rec.Code)
	}
	// Username matching is case-insensitive.
	if rec := login(h, "10.0.0.1", "ANA", "ok"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("case variant: expected 429, got %d", rec.Code)
	}
}

func TestLoginLimit_SuccessResetsCounter(t *testing.T) {
	h := middleware.LoginLimit(ratelimit.NewMemory(3, time.Minute))(fakeLogin(t))

	login(h, "10.0.0.1", "ana", "bad")
	login(h, "10.0.0.1", "ana", "bad")
	if rec := login(h, "10.0.0.1", "ana", "ok"); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	login(h, "10.0.0.1", "ana", "bad")
	login(h, "10.0.0.1", "ana", "bad")
	if rec := login(h, "10.0.0.1", "ana", "ok"); rec.Code != http.StatusOK {
		t.Errorf("counter must restart after success, got %d", rec.Code)
	}
}

func TestLoginLimit_HandlerReceivesFullBody(t *testing.T) {
	var got string
	h := middleware.LoginLimit(ratelimit.NewMemory(3, time.Minute))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	}))
	body := `{"username":"ana","password":"` + strings.Repeat("x", 100<<10) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != body {
		t.Errorf("handler must receive the original body (got %d bytes, want %d)", len(got), len(body))
	}
}

type brokenLimiter struct{}

var errDown = errors.New("redis down")

func (brokenLimiter) Blocked(context.Context, string) (bool, time.Duration, error) {
	return false, 0, errDown
}
func (brokenLimiter) Fail(context.Context, string) error  { return errDown }
func (brokenLimiter) Reset(context.Context, string) error { return errDown }

func TestLoginLimit_BackendDown_FailsOpen(t *testing.T) {
	h := middleware.LoginLimit(brokenLimiter{})(fakeLogin(t))
	if rec := login(h, "10.0.0.1", "ana", "ok"); rec.Code != http.StatusOK {
		t.Errorf("expected 200 when the limiter is down, got %d", rec.Code)
	}
	if rec := login(h, "10.0.0.1", "ana", "bad"); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when the limiter is down, got %d", rec.Code)
	}
}
