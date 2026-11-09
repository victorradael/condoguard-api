package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// DefaultMaxBodyBytes is the largest request body the API accepts.
const DefaultMaxBodyBytes = 1 << 20 // 1 MiB

// MaxBody rejects bodies larger than limit: with 413 up front when the
// Content-Length says so, and by failing the read (the handler then answers
// 400) when the body is streamed without a length.
func MaxBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets the response headers every API answer should carry.
// Responses are user data: they must not be cached by shared caches or
// sniffed into another content type, and the API is never framed.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// StaticToken protects an endpoint for machines (e.g. the metrics scraper)
// with "Authorization: Bearer <token>". An empty token disables the endpoint
// (404), so it is never public by accident.
func StaticToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				http.NotFound(w, r)
				return
			}
			got, ok := parseBearer(r.Header.Get("Authorization"))
			if !ok || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
				writeJSONError(w, http.StatusUnauthorized, "missing or invalid token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
