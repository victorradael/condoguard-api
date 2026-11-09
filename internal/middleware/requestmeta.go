package middleware

import (
	"context"
	"net/http"
)

type requestMetaKey struct{}

// RequestMeta is where a request came from, for the audit trail.
type RequestMeta struct {
	IP        string
	UserAgent string
}

const maxUserAgent = 256

// WithRequestMeta stores the client IP (as resolved by ClientIP) and the
// User-Agent in the request context. Install it after ClientIP.
func WithRequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.UserAgent()
		if len(ua) > maxUserAgent {
			ua = ua[:maxUserAgent]
		}
		m := RequestMeta{IP: clientIP(r), UserAgent: ua}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestMetaKey{}, m)))
	})
}

// RequestMetaFromContext returns what WithRequestMeta stored (zero when absent).
func RequestMetaFromContext(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return m
}

// ContextWithRequestMeta returns ctx carrying m. Meant for tests and internal callers.
func ContextWithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, m)
}
