package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"

	"github.com/victorradael/condoguard/api/pkg/ratelimit"
)

// loginBodyLimit is the largest login body accepted. Anything bigger is
// rejected with 400 before the handler runs.
const loginBodyLimit = 64 << 10

// LoginLimit locks out (client IP, username) pairs after too many failed
// logins. It wraps the login handler without changing it.
//
// Each attempt is reserved atomically before the handler runs, so concurrent
// requests can't all slip past the check while bcrypt is still working. A 401
// keeps the reservation as a failure, a 200 clears the counter, and any other
// status gives the reservation back. A locked-out pair gets 429 with
// Retry-After before the handler runs.
//
// The body must be a single JSON value of at most loginBodyLimit bytes;
// otherwise the middleware answers 400 itself. This guarantees the username
// used for the key is the one the handler authenticates, so a login to one
// account can never reset the counter of another.
//
// The client IP is taken from RemoteAddr only; X-Forwarded-For is not trusted
// until the reverse proxy (Caddy) is in place and configured as trusted.
// If the limiter backend fails, the request is let through and the error is
// logged: an unavailable Redis must not take logins down.
func LoginLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, ok := readLoginUsername(r)
			if !ok {
				writeJSONError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if username == "" {
				// The handler rejects it with 422; nothing to count.
				next.ServeHTTP(w, r)
				return
			}

			key := loginKey(r, username)
			ctx := r.Context()

			reserved, retryAfter, err := limiter.Reserve(ctx, key)
			if err != nil {
				slog.WarnContext(ctx, "login limiter unavailable", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !reserved {
				secs := int(math.Ceil(retryAfter.Seconds()))
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				writeJSONError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
				return
			}

			rec := &responseWriter{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			switch rec.status {
			case http.StatusUnauthorized:
				err = nil // the reserved attempt stays as a failure
			case http.StatusOK:
				err = limiter.Reset(ctx, key)
			default:
				err = limiter.Release(ctx, key)
			}
			if err != nil {
				slog.WarnContext(ctx, "login limiter unavailable", "error", err)
			}
		})
	}
}

// readLoginUsername reads the body, restores it for the handler and returns
// the username it contains. ok is false when the body is too large or is not
// exactly one JSON value with the login fields' types, i.e. whenever the
// handler could read a different username (or none) from it.
func readLoginUsername(r *http.Request) (username string, ok bool) {
	if r.Body == nil {
		return "", false
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, loginBodyLimit+1))
	r.Body = readCloser{bytes.NewReader(buf), r.Body}
	if err != nil || len(buf) > loginBodyLimit {
		return "", false
	}

	// Same shape and decoder as the login handler, plus a check that nothing
	// follows the first value.
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	dec := json.NewDecoder(bytes.NewReader(buf))
	if dec.Decode(&body) != nil {
		return "", false
	}
	var extra json.RawMessage
	if !errors.Is(dec.Decode(&extra), io.EOF) {
		return "", false
	}
	return body.Username, true
}

// loginKey identifies the (IP, username) pair without storing the raw username.
// The username is compared byte for byte, like the user lookup: "ana" and "ANA"
// are different accounts, so a login to one must never reset the other's counter.
func loginKey(r *http.Request, username string) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	sum := sha256.Sum256([]byte(username))
	return ip + ":" + hex.EncodeToString(sum[:16])
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type readCloser struct {
	io.Reader
	io.Closer
}
