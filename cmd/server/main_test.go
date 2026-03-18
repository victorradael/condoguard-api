package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/app"
	"github.com/victorradael/condoguard/api/internal/middleware"
)

func newTestRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := middleware.NewMetrics()
	return app.NewRouter(logger, metrics)
}

func TestHealthEndpoint_Returns200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestHealthEndpoint_ReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

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
	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestRouter_SetsRequestIDHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id header in response")
	}
}

func TestMetricsEndpoint_Returns200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /metrics, got %d", rec.Code)
	}
}

func TestOpenAPIEndpoint_Returns200WithSpec(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /openapi.json, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}

func TestDocsEndpoint_Returns200WithHTML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /docs, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
}
