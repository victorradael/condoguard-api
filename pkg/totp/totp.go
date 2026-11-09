// Package totp implements time-based one-time passwords (RFC 6238, with the
// HOTP of RFC 4226): HMAC-SHA1, 6 digits, 30-second steps, the parameters
// every authenticator app supports.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default; authenticator apps expect HMAC-SHA1
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Period is the length of one step.
	Period = 30 * time.Second
	// Digits is the length of a code.
	Digits = 6
	// SecretBytes is the size of a new secret (160 bits, as RFC 4226 recommends).
	SecretBytes = 20
	// Skew is how many steps before and after the current one are accepted, for clock drift.
	Skew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random secret in the Base32 form authenticator apps take.
func NewSecret() (string, error) {
	b := make([]byte, SecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// Step is the time step (counter) of t.
func Step(t time.Time) int64 { return t.Unix() / int64(Period.Seconds()) }

// Code returns the code of the secret at the given step.
func Code(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("totp: invalid secret")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // steps are positive
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", Digits, v%1_000_000), nil
}

// Verify checks code against the steps around now and returns the step it
// matched. Callers store the step and refuse any code whose step is not
// greater than the last accepted one, so a code works once.
func Verify(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != Digits {
		return 0, false
	}
	cur := Step(now)
	// Every candidate is compared, in constant time, so timing says nothing about which step matched.
	var matched int64
	found := 0
	for d := int64(-Skew); d <= Skew; d++ {
		want, err := Code(secret, cur+d)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			matched, found = cur+d, 1
		}
	}
	return matched, found == 1
}

// URL is the otpauth:// URI an authenticator app reads from a QR code.
func URL(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
