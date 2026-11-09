package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	body := `{"username":"ana","password":"` + strings.Repeat("x", 60<<10) + `"}` + "\n"
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != body {
		t.Errorf("handler must receive the original body (got %d bytes, want %d)", len(got), len(body))
	}
}

func TestLoginLimit_RejectsBodiesTheKeyCantMatch(t *testing.T) {
	cases := map[string]string{
		"trailing data": `{"username":"ana","password":"bad"} x`,
		"two values":    `{"username":"x"}{"username":"ana","password":"bad"}`,
		"not JSON":      `username=ana`,
		"empty":         ``,
		"wrong type":    `{"username":1,"password":"bad"}`,
		"too large":     `{"username":"ana","password":"` + strings.Repeat("x", 100<<10) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			h := middleware.LoginLimit(ratelimit.NewMemory(3, time.Minute))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest || called {
				t.Errorf("expected 400 without reaching the handler, got %d (handler called: %v)", rec.Code, called)
			}
			if !strings.Contains(rec.Body.String(), `"invalid JSON"`) {
				t.Errorf("expected the handler's error body, got %q", rec.Body.String())
			}
		})
	}
}

// Regression (FAB-11 B1): parallel attempts must not all pass the check while
// the slow handler (bcrypt) is still running.
func TestLoginLimit_ParallelAttemptsRespectMax(t *testing.T) {
	var reached atomic.Int64
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusUnauthorized)
	})
	h := middleware.LoginLimit(ratelimit.NewMemory(5, time.Minute))(slow)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			login(h, "10.0.0.1", "ana", "bad")
		}()
	}
	wg.Wait()
	if got := reached.Load(); got > 5 {
		t.Errorf("200 parallel attempts: %d reached the handler, max is 5", got)
	}
}

// Regression (FAB-11 B2): an attacker's own successful login must not reset
// the counter used against the victim, whatever the body looks like.
func TestLoginLimit_OwnLoginDoesNotResetVictimCounter(t *testing.T) {
	var victimAttempts int
	h := middleware.LoginLimit(ratelimit.NewMemory(5, time.Minute))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Username, Password string }
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.Username == "attacker" && req.Password == "ok" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if req.Username == "victim" {
			victimAttempts++
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	send := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.RemoteAddr = "10.0.0.9:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	got429 := false
	for i := 0; i < 100; i++ {
		for j := 0; j < 4; j++ {
			if send(`{"username":"victim","password":"guess"} x`) == http.StatusTooManyRequests ||
				send(`{"username":"victim","password":"guess"}`) == http.StatusTooManyRequests {
				got429 = true
			}
		}
		send(`{"username":"attacker","password":"ok"} x`)
		send(`{"username":"attacker","password":"ok"}`)
	}
	if victimAttempts > 5 || !got429 {
		t.Errorf("%d attempts against the victim reached the handler (max 5), 429 seen: %v", victimAttempts, got429)
	}
}

func TestLoginLimit_OtherStatusesDoNotCount(t *testing.T) {
	status := http.StatusUnprocessableEntity
	h := middleware.LoginLimit(ratelimit.NewMemory(2, time.Minute))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	for i := 0; i < 5; i++ {
		if rec := login(h, "10.0.0.1", "ana", ""); rec.Code != status {
			t.Fatalf("attempt %d: expected %d, got %d", i+1, status, rec.Code)
		}
	}
	status = http.StatusInternalServerError
	for i := 0; i < 5; i++ {
		if rec := login(h, "10.0.0.1", "ana", "x"); rec.Code != status {
			t.Fatalf("attempt %d: expected %d, got %d", i+1, status, rec.Code)
		}
	}
}

type brokenLimiter struct{}

var errDown = errors.New("redis down")

func (brokenLimiter) Reserve(context.Context, string) (bool, time.Duration, error) {
	return false, 0, errDown
}
func (brokenLimiter) Release(context.Context, string) error { return errDown }
func (brokenLimiter) Reset(context.Context, string) error   { return errDown }

func TestLoginLimit_BackendDown_FailsOpen(t *testing.T) {
	h := middleware.LoginLimit(brokenLimiter{})(fakeLogin(t))
	if rec := login(h, "10.0.0.1", "ana", "ok"); rec.Code != http.StatusOK {
		t.Errorf("expected 200 when the limiter is down, got %d", rec.Code)
	}
	if rec := login(h, "10.0.0.1", "ana", "bad"); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when the limiter is down, got %d", rec.Code)
	}
}
