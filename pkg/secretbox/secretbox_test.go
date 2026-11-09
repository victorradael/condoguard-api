package secretbox_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/victorradael/condoguard/api/pkg/secretbox"
)

func key(t *testing.T) string {
	t.Helper()
	b := make([]byte, secretbox.KeyBytes)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func TestSealOpen(t *testing.T) {
	box, err := secretbox.New(key(t))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := box.Seal([]byte("JBSWY3DPEHPK3PXP"), []byte("user-1"))
	b, _ := box.Seal([]byte("JBSWY3DPEHPK3PXP"), []byte("user-1"))
	if bytes.Equal(a, b) || bytes.Contains(a, []byte("JBSWY3DP")) {
		t.Error("sealing must be randomised and must not leak the plaintext")
	}
	if got, err := box.Open(a, []byte("user-1")); err != nil || string(got) != "JBSWY3DPEHPK3PXP" {
		t.Errorf("open: %q, %v", got, err)
	}
	if _, err := box.Open(a, []byte("user-2")); !errors.Is(err, secretbox.ErrDecrypt) {
		t.Errorf("another context: expected ErrDecrypt, got %v", err)
	}
	a[len(a)-1] ^= 1
	if _, err := box.Open(a, []byte("user-1")); !errors.Is(err, secretbox.ErrDecrypt) {
		t.Errorf("tampered: expected ErrDecrypt, got %v", err)
	}
	other, _ := secretbox.New(key(t))
	c, _ := box.Seal([]byte("x"), nil)
	if _, err := other.Open(c, nil); !errors.Is(err, secretbox.ErrDecrypt) {
		t.Errorf("another key: expected ErrDecrypt, got %v", err)
	}
	if _, err := box.Open([]byte("short"), nil); !errors.Is(err, secretbox.ErrDecrypt) {
		t.Errorf("short: expected ErrDecrypt, got %v", err)
	}
}

func TestNew_RejectsBadKeys(t *testing.T) {
	for _, k := range []string{"", "short", "!!!not base64!!!", base64.StdEncoding.EncodeToString(make([]byte, 16))} {
		if _, err := secretbox.New(k); !errors.Is(err, secretbox.ErrBadKey) {
			t.Errorf("%q: expected ErrBadKey, got %v", k, err)
		}
	}
}
