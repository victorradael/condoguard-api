package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

type contextKey string

const (
	userIDKey contextKey = "userID"
	rolesKey  contextKey = "roles"
)

// Cookie names used by the web client. Both are httpOnly: page scripts never
// see the tokens.
const (
	AccessCookie  = "cg_access"
	RefreshCookie = "cg_refresh"
)

// Authenticate returns a middleware that validates the access token sent as
// "Authorization: Bearer <token>" (app) or in the AccessCookie (web).
//
// On success it injects the user ID, the roles and the token's condominium
// (tenant) into the request context: from here on every query is scoped to
// the caller's condominium, whatever the deployment default is.
//
// Cookie-authenticated requests that change state must carry an Origin from
// allowedOrigins (CSRF protection); otherwise they get 403.
func Authenticate(jwtSvc *pkgjwt.Service, allowedOrigins ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, fromCookie, ok := extractToken(r)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "missing or malformed token")
				return
			}

			claims, err := jwtSvc.ValidateToken(token)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			if fromCookie && !CheckCookieOrigin(r, allowedOrigins) {
				writeJSONError(w, http.StatusForbidden, "origin not allowed")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, rolesKey, claims.Roles)
			ctx = tenant.WithCondominiumID(ctx, claims.CondominiumID)
			ctx = tenant.WithUserID(ctx, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole answers 403 unless the authenticated caller has role.
// It must run after Authenticate.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, have := range RolesFromContext(r.Context()) {
				if have == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeJSONError(w, http.StatusForbidden, "insufficient role")
		})
	}
}

// UserIDFromContext retrieves the authenticated user ID from the context.
// Returns empty string if not present.
func UserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey).(string)
	return id
}

// RolesFromContext retrieves the authenticated user roles from the context.
// Returns nil if not present.
func RolesFromContext(ctx context.Context) []string {
	roles, _ := ctx.Value(rolesKey).([]string)
	return roles
}

// WithIdentity returns ctx carrying an authenticated user, as Authenticate
// would set it. Meant for tests and internal callers (e.g. CLI tools).
func WithIdentity(ctx context.Context, userID string, roles []string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	return context.WithValue(ctx, rolesKey, roles)
}

// extractToken prefers the Authorization header; the cookie is the fallback.
// A present but malformed Authorization header is an error, not a fallback.
func extractToken(r *http.Request) (token string, fromCookie, ok bool) {
	if header := r.Header.Get("Authorization"); header != "" {
		t, ok := parseBearer(header)
		return t, false, ok
	}
	if c, err := r.Cookie(AccessCookie); err == nil && c.Value != "" {
		return c.Value, true, true
	}
	return "", false, false
}

func parseBearer(header string) (string, bool) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}
