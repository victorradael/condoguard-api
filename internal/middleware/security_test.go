package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

// ── CORS (item 6) ─────────────────────────────────────────────────────────────

func TestParseOrigins(t *testing.T) {
	got, err := middleware.ParseOrigins(" http://localhost:3000 , https://app.condoguard.com.br,")
	if err != nil || len(got) != 2 || got[1] != "https://app.condoguard.com.br" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []string{"*", "http://a.io,*", "localhost:3000", "http://a.io/", "http://a.io/path", "ftp://a.io", "http://user@a.io"} {
		if _, err := middleware.ParseOrigins(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func corsRequest(method, origin string) *http.Request {
	req := httptest.NewRequest(method, "/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestCORS_AllowedOriginIsEchoedWithCredentials(t *testing.T) {
	h := middleware.CORS([]string{"http://localhost:3000"})(protectedHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, corsRequest(http.MethodGet, "http://localhost:3000"))

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("expected Allow-Credentials")
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Error("expected Vary: Origin")
	}
	// The web reads Retry-After on 429 and quotes the request ID in errors.
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "Retry-After, X-Request-Id" {
		t.Errorf("Expose-Headers = %q", got)
	}
}

// An origin outside the list never gets CORS headers, and the response is
// never "*" — whatever the request says.
func TestCORS_OtherOriginGetsNoHeaders(t *testing.T) {
	h := middleware.CORS([]string{"http://localhost:3000"})(protectedHandler())
	for _, origin := range []string{"https://evil.example", "null", "http://localhost:3000.evil.example", "*"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, corsRequest(http.MethodGet, origin))
		if v := rec.Header().Get("Access-Control-Allow-Origin"); v != "" {
			t.Errorf("%q: Allow-Origin = %q, want none", origin, v)
		}
		if v := rec.Header().Get("Access-Control-Allow-Credentials"); v != "" {
			t.Errorf("%q: Allow-Credentials = %q, want none", origin, v)
		}
		if v := rec.Header().Get("Access-Control-Expose-Headers"); v != "" {
			t.Errorf("%q: Expose-Headers = %q, want none", origin, v)
		}
	}
}

func TestCORS_Preflight(t *testing.T) {
	h := middleware.CORS([]string{"http://localhost:3000"})(protectedHandler())

	ok := corsRequest(http.MethodOptions, "http://localhost:3000")
	ok.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, ok)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("allowed preflight: %d %v", rec.Code, rec.Header())
	}

	bad := corsRequest(http.MethodOptions, "https://evil.example")
	bad.Header.Set("Access-Control-Request-Method", "POST")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("foreign preflight: %d %v", rec.Code, rec.Header())
	}
}

// ── Body size limit (item 7) ──────────────────────────────────────────────────

func TestMaxBody(t *testing.T) {
	var readErr error
	h := middleware.MaxBody(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("0123456789abc")))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("declared length over limit: expected 413, got %d", rec.Code)
	}

	// Streamed body without a length: the read fails once over the limit.
	req := httptest.NewRequest(http.MethodPost, "/x", io.NopCloser(strings.NewReader("0123456789abc")))
	req.ContentLength = -1
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if readErr == nil || rec.Code != http.StatusBadRequest {
		t.Errorf("streamed body over limit: err %v, status %d", readErr, rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("0123456789")))
	if rec.Code != http.StatusOK || readErr != nil {
		t.Errorf("body at the limit: %d %v", rec.Code, readErr)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	middleware.SecurityHeaders(protectedHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for h, want := range map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Cache-Control": "no-store",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
		"Strict-Transport-Security": "max-age=31536000",
	} {
		if got := rec.Header().Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
}

// ── Static token (/metrics) ───────────────────────────────────────────────────

func TestStaticToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	h := middleware.StaticToken(token)(protectedHandler())
	for header, want := range map[string]int{
		"":                      http.StatusUnauthorized,
		"Bearer wrong":          http.StatusUnauthorized,
		"Bearer " + token + "x": http.StatusUnauthorized,
		"Bearer " + token:       http.StatusOK,
	} {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%q: expected %d, got %d", header, want, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	middleware.StaticToken("")(protectedHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("no token configured: expected 404, got %d", rec.Code)
	}
}
