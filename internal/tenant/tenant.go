// Package tenant carries the current condominium (tenant) through the request
// context. Every repository query is scoped by the condominium ID read from
// here; a context without a tenant is rejected (fail closed).
//
// The tenant comes from the authenticated user's token (see
// middleware.Authenticate). Before authentication (sign-up, login) it is the
// deployment default configured via DEFAULT_CONDOMINIUM_ID, when there is one.
// The same ID is handed to PostgreSQL per transaction (see db.Scoped), where
// Row-Level Security applies it a second time.
package tenant

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
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

// UUID returns the condominium ID stored in ctx as a uuid, for the PostgreSQL
// repositories. Anything that is not a canonical uuid counts as missing: a
// tenant that cannot match a row must not reach a query.
func UUID(ctx context.Context) (uuid.UUID, error) {
	id, err := CondominiumID(ctx)
	if err != nil {
		return uuid.UUID{}, err
	}
	if len(id) != 36 {
		return uuid.UUID{}, ErrMissing
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.UUID{}, ErrMissing
	}
	return u, nil
}

type userKey struct{}

// WithUserID returns a copy of ctx carrying the authenticated user's ID. It is
// handed to PostgreSQL next to the tenant, so a user can always read their own
// account row.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey{}, userID)
}

// UserID returns the authenticated user's ID stored in ctx, or "".
func UserID(ctx context.Context) string {
	id, _ := ctx.Value(userKey{}).(string)
	return id
}

// Middleware stores condominiumID in every request context.
func Middleware(condominiumID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if condominiumID == "" {
				next.ServeHTTP(w, r) // no default condominium: no tenant until the token gives one
				return
			}
			next.ServeHTTP(w, r.WithContext(WithCondominiumID(r.Context(), condominiumID)))
		})
	}
}
