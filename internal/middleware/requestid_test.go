package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

func TestRequestID_InjectsHeaderInResponse(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	middleware.RequestID(next).ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id header in response")
	}
}

func TestRequestID_ReusesExistingRequestID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "my-custom-id")
	rec := httptest.NewRecorder()
	middleware.RequestID(next).ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-Id") != "my-custom-id" {
		t.Errorf("expected reused request ID 'my-custom-id', got %q", rec.Header().Get("X-Request-Id"))
	}
}

func TestRequestID_GeneratesUniqueIDs(t *testing.T) {
	ids := make(map[string]struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		middleware.RequestID(next).ServeHTTP(rec, req)
		id := rec.Header().Get("X-Request-Id")
		if _, seen := ids[id]; seen {
			t.Errorf("duplicate request ID generated: %q", id)
		}
		ids[id] = struct{}{}
	}
}

func TestRequestID_ExposesIDInContext(t *testing.T) {
	var capturedID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	middleware.RequestID(next).ServeHTTP(rec, req)

	if capturedID == "" {
		t.Error("expected request ID in context")
	}
	if capturedID != rec.Header().Get("X-Request-Id") {
		t.Errorf("context ID %q != response header %q", capturedID, rec.Header().Get("X-Request-Id"))
	}
}

// A client-sent ID is only reused when it is short and plain; anything else
// is replaced, so it can't bloat or forge the logs.
func TestRequestID_RejectsUnsafeIDs(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	for _, id := range []string{
		strings.Repeat("a", 65),
		"id with spaces",
		"id\nfake-log-line",
		"<script>",
		"ação",
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-Id", id)
		rec := httptest.NewRecorder()
		middleware.RequestID(next).ServeHTTP(rec, req)
		got := rec.Header().Get("X-Request-Id")
		if got == id || got == "" {
			t.Errorf("%q: expected a generated ID, got %q", id, got)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", strings.Repeat("A1-", 21)+"z")
	rec := httptest.NewRecorder()
	middleware.RequestID(next).ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-Id"); got != strings.Repeat("A1-", 21)+"z" {
		t.Errorf("a 64-char plain ID must be kept, got %q", got)
	}
}
