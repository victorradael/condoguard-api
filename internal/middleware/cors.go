package middleware

import (
	"net/http"
)

// CORS returns a middleware that answers cross-origin requests from the
// allowed origins only (see ParseOrigins). Credentials are allowed, so the
// origin is always echoed exactly, never as "*".
func CORS(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Origin")
			ok := originAllowed(r, allowed)
			if ok {
				w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				// Lets the web read the 429 back-off and quote the request ID.
				w.Header().Set("Access-Control-Expose-Headers", "Retry-After, X-Request-Id")
			}

			// Preflight
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if !ok {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
