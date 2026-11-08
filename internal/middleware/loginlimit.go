package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/victorradael/condoguard/api/pkg/ratelimit"
)

// loginPeekLimit is how much of the body is buffered to read the username.
// Larger bodies are passed through untouched and counted by IP only.
const loginPeekLimit = 64 << 10

// LoginLimit locks out (client IP, username) pairs after too many failed
// logins. It wraps the login handler without changing it: a 401 from the
// handler counts as a failure, a 200 clears the counter, and a locked-out
// pair gets 429 with Retry-After before the handler runs.
//
// The client IP is taken from RemoteAddr only; X-Forwarded-For is not trusted
// until the reverse proxy (Caddy) is in place and configured as trusted.
// If the limiter backend fails, the request is let through and the error is
// logged: an unavailable Redis must not take logins down.
func LoginLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := loginKey(r)
			ctx := r.Context()

			blocked, retryAfter, err := limiter.Blocked(ctx, key)
			if err != nil {
				slog.WarnContext(ctx, "login limiter unavailable", "error", err)
			}
			if blocked {
				secs := int(math.Ceil(retryAfter.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too many login attempts, try again later"}` + "\n"))
				return
			}

			rec := &responseWriter{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			switch rec.status {
			case http.StatusUnauthorized:
				err = limiter.Fail(ctx, key)
			case http.StatusOK:
				err = limiter.Reset(ctx, key)
			default:
				err = nil
			}
			if err != nil {
				slog.WarnContext(ctx, "login limiter unavailable", "error", err)
			}
		})
	}
}

// loginKey identifies the (IP, username) pair without storing the raw username.
// The request body is restored so the handler reads it unchanged.
func loginKey(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}

	username := ""
	if r.Body != nil {
		buf, _ := io.ReadAll(io.LimitReader(r.Body, loginPeekLimit+1))
		r.Body = readCloser{io.MultiReader(bytes.NewReader(buf), r.Body), r.Body}
		if len(buf) <= loginPeekLimit {
			var body struct {
				Username string `json:"username"`
			}
			if json.Unmarshal(buf, &body) == nil {
				username = strings.ToLower(body.Username)
			}
		}
	}

	sum := sha256.Sum256([]byte(username))
	return ip + ":" + hex.EncodeToString(sum[:16])
}

type readCloser struct {
	io.Reader
	io.Closer
}
