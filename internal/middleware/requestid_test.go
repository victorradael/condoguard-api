package middleware_test

import (
	"net/http"
	"net/http/httptest"
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
