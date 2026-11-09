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
	"strings"
	"time"

	"github.com/victorradael/condoguard/api/pkg/ratelimit"
)

// loginBodyLimit is the largest login body accepted. Anything bigger is
// rejected with 400 before the handler runs.
const loginBodyLimit = 64 << 10

// LoginLimit locks out (client IP, e-mail) pairs after too many failed
// logins and, when ipLimiter is not nil, client IPs after too many failed
// logins across any accounts (credential stuffing). It wraps the login
// handler without changing it.
//
// Each attempt is reserved atomically before the handler runs, so concurrent
// requests can't all slip past the check while bcrypt is still working. A 401
// keeps the reservations as failures; a 200 clears the pair counter and gives
// the IP reservation back (a successful login never resets the IP counter, so
// logging into one's own account does not unlock guessing on others); any
// other status gives the reservations back. A locked-out pair or IP gets 429
// with Retry-After before the handler runs.
//
// The body must be a single JSON value of at most loginBodyLimit bytes;
// otherwise the middleware answers 400 itself. This guarantees the e-mail
// used for the key is the one the handler authenticates, so a login to one
// account can never reset the counter of another.
//
// The client IP is taken from RemoteAddr only; X-Forwarded-For is not trusted
// until the reverse proxy (Caddy) is in place and configured as trusted.
// If the limiter backend fails, the request is let through and the error is
// logged: an unavailable Redis must not take logins down.
func LoginLimit(pairLimiter, ipLimiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			email, ok := readLoginEmail(r)
			if !ok {
				writeJSONError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if email == "" {
				// The handler rejects it with 422; nothing to count.
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			pair := reservation{limiter: pairLimiter, key: ip + ":" + emailHash(email)}
			var byIP reservation
			if ipLimiter != nil {
				byIP = reservation{limiter: ipLimiter, key: ip}
			}

			if retry, locked := byIP.reserve(r); locked {
				tooMany(w, retry)
				return
			}
			if retry, locked := pair.reserve(r); locked {
				byIP.release(r)
				tooMany(w, retry)
				return
			}

			rec := &responseWriter{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			switch rec.status {
			case http.StatusUnauthorized:
				// the reserved attempts stay as failures
			case http.StatusOK:
				pair.reset(r)
				byIP.release(r)
			default:
				pair.release(r)
				byIP.release(r)
			}
		})
	}
}

// reservation is one counted attempt in one limiter. A zero reservation
// (no limiter) is a no-op, and so is one whose backend failed to reserve.
type reservation struct {
	limiter ratelimit.Limiter
	key     string
	held    bool
}

// reserve counts the attempt. locked is true when the key is locked out.
func (v *reservation) reserve(r *http.Request) (retryAfter time.Duration, locked bool) {
	if v.limiter == nil {
		return 0, false
	}
	ok, retry, err := v.limiter.Reserve(r.Context(), v.key)
	if err != nil {
		slog.WarnContext(r.Context(), "login limiter unavailable", "error", err)
		return 0, false
	}
	if !ok {
		return retry, true
	}
	v.held = true
	return 0, false
}

func (v *reservation) release(r *http.Request) {
	if v.held {
		logLimiterErr(r, v.limiter.Release(r.Context(), v.key))
	}
}

func (v *reservation) reset(r *http.Request) {
	if v.held {
		logLimiterErr(r, v.limiter.Reset(r.Context(), v.key))
	}
}

func logLimiterErr(r *http.Request, err error) {
	if err != nil {
		slog.WarnContext(r.Context(), "login limiter unavailable", "error", err)
	}
}

func tooMany(w http.ResponseWriter, retryAfter time.Duration) {
	secs := max(int(math.Ceil(retryAfter.Seconds())), 1)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSONError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
}

// readLoginEmail reads the body, restores it for the handler and returns
// the e-mail it contains. ok is false when the body is too large or is not
// exactly one JSON value with the login fields' types, i.e. whenever the
// handler could read a different e-mail (or none) from it.
func readLoginEmail(r *http.Request) (email string, ok bool) {
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
		Email    string `json:"email"`
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
	return body.Email, true
}

func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// emailHash identifies the account without storing the raw e-mail. It is
// normalised exactly like the login lookup (trimmed, lower case), so "Ana@x"
// and "ana@x" share one counter, as they are one account.
func emailHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:16])
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
