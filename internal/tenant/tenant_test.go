package tenant_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestCondominiumID_Missing(t *testing.T) {
	if _, err := tenant.CondominiumID(context.Background()); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("expected ErrMissing, got %v", err)
	}
	if _, err := tenant.CondominiumID(tenant.WithCondominiumID(context.Background(), "")); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("empty ID must count as missing, got %v", err)
	}
}

func TestMiddleware_SetsCondominiumID(t *testing.T) {
	var got string
	h := tenant.Middleware("condo-1")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = tenant.CondominiumID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Condominium-Id", "attacker-choice")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "condo-1" {
		t.Errorf("expected configured tenant, got %q", got)
	}
}
