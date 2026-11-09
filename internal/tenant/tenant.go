// Package tenant carries the current condominium (tenant) through the request
// context. Every repository query is scoped by the condominium ID read from
// here; a context without a tenant is rejected (fail closed).
//
// Until the condominium entity exists (Fase 1b), the tenant is a single
// deployment-wide value configured via DEFAULT_CONDOMINIUM_ID. In 1b it will
// come from the authenticated user's token instead.
package tenant

import (
	"context"
	"errors"
	"net/http"
)

// ErrMissing is returned when a repository is called without a tenant in the context.
var ErrMissing = errors.New("tenant: condominium ID missing from context")

type ctxKey struct{}

// WithCondominiumID returns a copy of ctx carrying the given condominium ID.
func WithCondominiumID(ctx context.Context, condominiumID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, condominiumID)
}

// CondominiumID returns the condominium ID stored in ctx, or ErrMissing.
func CondominiumID(ctx context.Context) (string, error) {
	id, _ := ctx.Value(ctxKey{}).(string)
	if id == "" {
		return "", ErrMissing
	}
	return id, nil
}

// Middleware stores condominiumID in every request context.
func Middleware(condominiumID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithCondominiumID(r.Context(), condominiumID)))
		})
	}
}
