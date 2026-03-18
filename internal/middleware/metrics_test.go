package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

func TestMetrics_IncrementsRequestCount(t *testing.T) {
	m := middleware.NewMetrics()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.Middleware(next)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if m.TotalRequests() != 3 {
		t.Errorf("expected 3 total requests, got %d", m.TotalRequests())
	}
}

func TestMetrics_CountsErrorResponses(t *testing.T) {
	m := middleware.NewMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if m.ErrorRequests() != 2 {
		t.Errorf("expected 2 error responses, got %d", m.ErrorRequests())
	}
}

func TestMetrics_DoesNotCountSuccessAsError(t *testing.T) {
	m := middleware.NewMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if m.ErrorRequests() != 0 {
		t.Errorf("expected 0 errors for 200 response, got %d", m.ErrorRequests())
	}
}

func TestMetrics_TracksLatency(t *testing.T) {
	m := middleware.NewMetrics()

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if m.TotalLatencyMs() < 0 {
		t.Error("expected non-negative total latency")
	}
}
