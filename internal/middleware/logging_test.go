package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

func TestLogging_LogsMethodPathAndStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/test-path", nil)
	rec := httptest.NewRecorder()
	middleware.Logging(logger)(next).ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("expected valid JSON log entry, got: %s", buf.String())
	}

	if entry["method"] != "POST" {
		t.Errorf("expected method 'POST', got %v", entry["method"])
	}
	if entry["path"] != "/test-path" {
		t.Errorf("expected path '/test-path', got %v", entry["path"])
	}
	if entry["status"] != float64(201) {
		t.Errorf("expected status 201, got %v", entry["status"])
	}
}

func TestLogging_LogsDurationField(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	middleware.Logging(logger)(next).ServeHTTP(rec, req)

	var entry map[string]any
	_ = json.NewDecoder(&buf).Decode(&entry)

	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected duration_ms field in log entry")
	}
}

func TestLogging_IncludesRequestIDWhenPresent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Chain RequestID then Logging so the ID is in context.
	handler := middleware.RequestID(middleware.Logging(logger)(next))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	_ = json.NewDecoder(&buf).Decode(&entry)

	if entry["request_id"] == nil || entry["request_id"] == "" {
		t.Error("expected request_id in log entry when RequestID middleware is active")
	}
}
