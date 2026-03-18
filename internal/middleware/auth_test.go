package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/middleware"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const testSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="

func protectedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected"))
	})
}

func newMiddleware() func(http.Handler) http.Handler {
	jwtSvc := pkgjwt.NewService(testSecret)
	return middleware.Authenticate(jwtSvc)
}

func bearerRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// ── Missing token ─────────────────────────────────────────────────────────────

func TestMiddleware_NoAuthHeader_Returns401(t *testing.T) {
	mw := newMiddleware()
	handler := mw(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_EmptyBearerToken_Returns401(t *testing.T) {
	mw := newMiddleware()
	handler := mw(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_MalformedHeader_Returns401(t *testing.T) {
	mw := newMiddleware()
	handler := mw(protectedHandler())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token abc123") // not Bearer
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── Invalid token ─────────────────────────────────────────────────────────────

func TestMiddleware_InvalidToken_Returns401(t *testing.T) {
	mw := newMiddleware()
	handler := mw(protectedHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, bearerRequest("not.a.valid.jwt"))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_TokenSignedWithWrongSecret_Returns401(t *testing.T) {
	otherSvc := pkgjwt.NewService("b3RoZXItc2VjcmV0LWtleQ==")
	token, _ := otherSvc.GenerateToken("user-123", []string{"ROLE_USER"})

	mw := newMiddleware()
	handler := mw(protectedHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, bearerRequest(token))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── Expired token ─────────────────────────────────────────────────────────────

func TestMiddleware_ExpiredToken_Returns401(t *testing.T) {
	expiredSvc := pkgjwt.NewServiceWithExpiry(testSecret, -1*time.Hour)
	token, _ := expiredSvc.GenerateToken("user-123", []string{"ROLE_USER"})

	mw := newMiddleware()
	handler := mw(protectedHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, bearerRequest(token))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── Valid token ───────────────────────────────────────────────────────────────

func TestMiddleware_ValidToken_PassesThrough(t *testing.T) {
	jwtSvc := pkgjwt.NewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-123", []string{"ROLE_USER"})

	mw := newMiddleware()
	handler := mw(protectedHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, bearerRequest(token))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_ValidToken_UserIDInContext(t *testing.T) {
	jwtSvc := pkgjwt.NewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-abc", []string{"ROLE_USER"})

	mw := newMiddleware()
	var capturedUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = middleware.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, bearerRequest(token))

	if capturedUserID != "user-abc" {
		t.Errorf("expected userID 'user-abc' in context, got %q", capturedUserID)
	}
}
