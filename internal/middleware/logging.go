package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// Logging returns a middleware that emits a structured slog entry for every
// request, including method, path, status, duration_ms, and request_id (when
// the RequestID middleware has already populated the context).
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &responseWriter{ResponseWriter: w, status: 0}

			next.ServeHTTP(wrapped, r)

			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}

			args := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
			}

			if id := RequestIDFromContext(r.Context()); id != "" {
				args = append(args, "request_id", id)
			}

			logger.Info("request", args...)
		})
	}
}
