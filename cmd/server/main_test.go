package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/app"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const (
	testSecret       = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"
	testMetricsToken = "metrics-token-for-tests-0123456789abcdef"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	for k, v := range map[string]string{
		"DATABASE_URL":   "",
		"REDIS_URL":      "",
		"JWT_SECRET_KEY": testSecret,
		"METRICS_TOKEN":  testMetricsToken,
	} {
		t.Setenv(k, v)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := middleware.NewMetrics()
	return app.NewRouter(logger, metrics)
}

func get(t *testing.T, h http.Handler, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func tokenWithRole(t *testing.T, role string) string {
	t.Helper()
	tok, err := pkgjwt.MustNewService(testSecret).GenerateToken("user-1", "default", []string{role})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestHealthEndpoint_Returns200(t *testing.T) {
	if rec := get(t, newTestRouter(t), "/health", ""); rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestHealthEndpoint_ReturnsJSON(t *testing.T) {
	rec := get(t, newTestRouter(t), "/health", "")

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	if body := rec.Body.String(); body != `{"status":"ok"}` {
		t.Errorf("expected body {\"status\":\"ok\"}, got %q", body)
	}
}

func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestRouter_SetsRequestIDHeader(t *testing.T) {
	if rec := get(t, newTestRouter(t), "/health", ""); rec.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id header in response")
	}
}

// ── /metrics: METRICS_TOKEN bearer only ──────────────────────────────────────

func TestMetricsEndpoint_Security_RequiresMetricsToken(t *testing.T) {
	h := newTestRouter(t)
	for name, bearer := range map[string]string{
		"anonymous":   "",
		"wrong token": "not-the-metrics-token",
		"user JWT":    tokenWithRole(t, authz.RoleSindico),
	} {
		if rec := get(t, h, "/metrics", bearer); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401 from /metrics, got %d", name, rec.Code)
		}
	}
	if rec := get(t, h, "/metrics", testMetricsToken); rec.Code != http.StatusOK {
		t.Errorf("with METRICS_TOKEN: expected 200, got %d", rec.Code)
	}
}

func TestMetricsEndpoint_DisabledWithoutToken(t *testing.T) {
	_ = newTestRouter(t) // sets the rest of the environment
	t.Setenv("METRICS_TOKEN", "")
	h := app.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics())
	if rec := get(t, h, "/metrics", ""); rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when METRICS_TOKEN is unset, got %d", rec.Code)
	}
}

// ── /openapi.json and /docs: admins only ─────────────────────────────────────

func TestDocsEndpoints_Security_AdminOnly(t *testing.T) {
	h := newTestRouter(t)
	user := tokenWithRole(t, authz.RoleMorador)
	for _, path := range []string{"/openapi.json", "/docs"} {
		if rec := get(t, h, path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s: expected 401, got %d", path, rec.Code)
		}
		if rec := get(t, h, path, user); rec.Code != http.StatusForbidden {
			t.Errorf("morador %s: expected 403, got %d", path, rec.Code)
		}
	}
}

func TestOpenAPIEndpoint_Admin_Returns200WithSpec(t *testing.T) {
	rec := get(t, newTestRouter(t), "/openapi.json", tokenWithRole(t, authz.RoleSindico))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /openapi.json, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}

func TestDocsEndpoint_Admin_Returns200WithHTML(t *testing.T) {
	rec := get(t, newTestRouter(t), "/docs", tokenWithRole(t, authz.RoleSindico))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /docs, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
}
