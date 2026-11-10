package totp_test

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/pkg/totp"
)

// RFC 6238 appendix B, SHA-1 vectors (8 digits there; the last 6 are ours).
func TestCode_RFC6238Vectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for ts, want := range map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"} {
		got, err := totp.Code(secret, totp.Step(time.Unix(ts, 0)))
		if err != nil || got != want {
			t.Errorf("t=%d: got %q (%v), want %q", ts, got, err, want)
		}
	}
}

func TestVerify_WindowAndFormat(t *testing.T) {
	secret, err := totp.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	cur := totp.Step(now)
	at := func(step int64) string { c, _ := totp.Code(secret, step); return c }

	for d, ok := range map[int64]bool{-2: false, -1: true, 0: true, 1: true, 2: false} {
		step, got := totp.Verify(secret, at(cur+d), now)
		if got != ok || (ok && step != cur+d) {
			t.Errorf("step %+d: ok=%v step=%d, want ok=%v", d, got, step, ok)
		}
	}
	// Spaces are tolerated (apps show "123 456"); anything else is not a code.
	c := at(cur)
	if _, ok := totp.Verify(secret, c[:3]+" "+c[3:], now); !ok {
		t.Error("a code with a space must verify")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "000000 0"} {
		if _, ok := totp.Verify(secret, bad, now); ok && bad != at(cur) {
			t.Errorf("%q must not verify", bad)
		}
	}
	if _, ok := totp.Verify("not base32 !!", c, now); ok {
		t.Error("an invalid secret must not verify")
	}
}

func TestNewSecretAndURL(t *testing.T) {
	a, _ := totp.NewSecret()
	b, _ := totp.NewSecret()
	if a == b || len(a) != 32 {
		t.Errorf("secrets must be random 160-bit Base32: %q %q", a, b)
	}
	u := totp.URL("Condoguard", "ana@exemplo.test", a)
	for _, want := range []string{"otpauth://totp/Condoguard:ana@exemplo.test?", "secret=" + a, "issuer=Condoguard", "digits=6", "period=30"} {
		if !strings.Contains(u, want) {
			t.Errorf("URL %q lacks %q", u, want)
		}
	}
}
