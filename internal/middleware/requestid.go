package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const requestIDKey contextKey = "requestID"

// RequestID is a middleware that reads X-Request-Id from the incoming request
// or generates a random one. The ID is stored in the context and echoed back
// in the response header. A client ID that is not 1–64 letters, digits or
// dashes is replaced, so it can't bloat or garble every log line.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !validRequestID(id) {
			id = generateID()
		}

		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext retrieves the request ID from the context.
// Returns empty string if not present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
