package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const testSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"

func protectedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected"))
	})
}

func newMiddleware() func(http.Handler) http.Handler {
	jwtSvc := pkgjwt.MustNewService(testSecret)
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
	otherSvc := pkgjwt.MustNewService("b3RoZXItc2VjcmV0LWtleS1mb3ItdGVzdGluZy0zMmJ5dGVz")
	token, _ := otherSvc.GenerateToken("user-123", "condo-a", []string{"morador"})

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
	expiredSvc, _ := pkgjwt.NewServiceWithExpiry(testSecret, -1*time.Hour)
	token, _ := expiredSvc.GenerateToken("user-123", "condo-a", []string{"morador"})

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
	jwtSvc := pkgjwt.MustNewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-123", "condo-a", []string{"morador"})

	mw := newMiddleware()
	handler := mw(protectedHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, bearerRequest(token))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_ValidToken_UserIDInContext(t *testing.T) {
	jwtSvc := pkgjwt.MustNewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-abc", "condo-a", []string{"morador"})

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

// ── Tenant from the token (item 4) ─────────────────────────────────────────────

// The condominium in the token replaces whatever tenant the request had (the
// deployment default), so an authenticated user only ever queries their own.
func TestMiddleware_TenantComesFromToken(t *testing.T) {
	jwtSvc := pkgjwt.MustNewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-1", "condo-b", []string{"morador"})

	var got string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = tenant.CondominiumID(r.Context())
	})
	h := tenant.Middleware("default")(middleware.Authenticate(jwtSvc)(next))
	h.ServeHTTP(httptest.NewRecorder(), bearerRequest(token))

	if got != "condo-b" {
		t.Errorf("expected tenant condo-b from the token, got %q", got)
	}
}

// ── Cookie authentication (item 8) ─────────────────────────────────────────────

func cookieRequest(method, token, origin string) *http.Request {
	req := httptest.NewRequest(method, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: middleware.AccessCookie, Value: token})
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return req
}

func TestMiddleware_Cookie_GetPassesThrough(t *testing.T) {
	jwtSvc := pkgjwt.MustNewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-1", "condo-a", []string{"morador"})
	h := middleware.Authenticate(jwtSvc, "http://localhost:3000")(protectedHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, cookieRequest(http.MethodGet, token, ""))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// CSRF: a state-changing request authenticated by cookie must come from an
// allowed origin; a bearer token is not sent automatically by browsers, so it
// is not checked.
func TestMiddleware_Cookie_UnsafeMethodNeedsAllowedOrigin(t *testing.T) {
	jwtSvc := pkgjwt.MustNewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-1", "condo-a", []string{"morador"})
	h := middleware.Authenticate(jwtSvc, "http://localhost:3000")(protectedHandler())

	for origin, want := range map[string]int{
		"http://localhost:3000": http.StatusOK,
		"https://evil.example":  http.StatusForbidden,
		"":                      http.StatusForbidden,
		"null":                  http.StatusForbidden,
	} {
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, cookieRequest(m, token, origin))
			if rec.Code != want {
				t.Errorf("%s from %q: expected %d, got %d", m, origin, want, rec.Code)
			}
		}
	}

	req := bearerRequest(token)
	req.Method = http.MethodPost
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("bearer POST: expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_Cookie_InvalidTokenReturns401(t *testing.T) {
	h := newMiddleware()(protectedHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, cookieRequest(http.MethodGet, "not.a.jwt", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// A malformed Authorization header is an error, never a silent fallback to the cookie.
func TestMiddleware_MalformedHeaderDoesNotFallBackToCookie(t *testing.T) {
	jwtSvc := pkgjwt.MustNewService(testSecret)
	token, _ := jwtSvc.GenerateToken("user-1", "condo-a", []string{"morador"})
	req := cookieRequest(http.MethodGet, token, "")
	req.Header.Set("Authorization", "Basic abc")
	rec := httptest.NewRecorder()
	newMiddleware()(protectedHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── RequireRole ───────────────────────────────────────────────────────────────

func TestRequireRole(t *testing.T) {
	jwtSvc := pkgjwt.MustNewService(testSecret)
	h := middleware.Authenticate(jwtSvc)(middleware.RequireRole("sindico")(protectedHandler()))
	for roles, want := range map[string]int{"sindico": http.StatusOK, "morador": http.StatusForbidden} {
		token, _ := jwtSvc.GenerateToken("u", "condo-a", []string{roles})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, bearerRequest(token))
		if rec.Code != want {
			t.Errorf("%s: expected %d, got %d", roles, want, rec.Code)
		}
	}
}
