package middleware

import (
	"context"
	"net/http"
	"strings"

	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

type contextKey string

const (
	userIDKey contextKey = "userID"
	rolesKey  contextKey = "roles"
)

// Authenticate returns a middleware that validates the JWT in the
// Authorization: Bearer <token> header.
// On success it injects the user ID into the request context.
// On failure it responds with 401 and does not call the next handler.
func Authenticate(jwtSvc *pkgjwt.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := extractBearer(r)
			if !ok {
				http.Error(w, `{"error":"missing or malformed token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtSvc.ValidateToken(token)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, rolesKey, claims.Roles)
			next.ServeHTTP(w, r.WithContext(ctx))
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

func extractBearer(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
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
