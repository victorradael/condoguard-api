package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORS returns a middleware that handles cross-origin requests.
// Allowed origins are read from the CORS_ALLOWED_ORIGINS environment variable
// (comma-separated). If not set, defaults to http://localhost:3000.
// In all cases "*" can be used to allow any origin.
func CORS(next http.Handler) http.Handler {
	raw := os.Getenv("CORS_ALLOWED_ORIGINS")
	if raw == "" {
		raw = "http://localhost:3000"
	}

	allowed := make(map[string]bool)
	for _, o := range strings.Split(raw, ",") {
		allowed[strings.TrimSpace(o)] = true
	}
	allowAll := allowed["*"]

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" && (allowAll || allowed[origin]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
