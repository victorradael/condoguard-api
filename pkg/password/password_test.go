package password_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/pkg/password"
)

// ── Hash ─────────────────────────────────────────────────────────────────────

func TestPassword_Hash_ReturnsNonEmptyString(t *testing.T) {
	hash, err := password.Hash("mysecret")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

func TestPassword_Hash_DifferentFromPlaintext(t *testing.T) {
	plain := "mysecret"
	hash, _ := password.Hash(plain)

	if hash == plain {
		t.Error("hash must differ from plaintext")
	}
}

func TestPassword_Hash_SamePlaintextProducesDifferentHashes(t *testing.T) {
	hash1, _ := password.Hash("mysecret")
	hash2, _ := password.Hash("mysecret")

	if hash1 == hash2 {
		t.Error("bcrypt must produce unique salts per call")
	}
}

func TestPassword_Hash_EmptyPasswordReturnsError(t *testing.T) {
	_, err := password.Hash("")

	if err == nil {
		t.Fatal("expected error for empty password, got nil")
	}
}

// ── Verify ───────────────────────────────────────────────────────────────────

func TestPassword_Verify_CorrectPasswordReturnsTrue(t *testing.T) {
	plain := "mysecret"
	hash, _ := password.Hash(plain)

	ok := password.Verify(plain, hash)
	if !ok {
		t.Error("expected Verify to return true for correct password")
	}
}

func TestPassword_Verify_WrongPasswordReturnsFalse(t *testing.T) {
	hash, _ := password.Hash("mysecret")

	ok := password.Verify("wrongpassword", hash)
	if ok {
		t.Error("expected Verify to return false for wrong password")
	}
}

func TestPassword_Verify_EmptyPasswordReturnsFalse(t *testing.T) {
	hash, _ := password.Hash("mysecret")

	ok := password.Verify("", hash)
	if ok {
		t.Error("expected Verify to return false for empty password")
	}
}

func TestPassword_Verify_EmptyHashReturnsFalse(t *testing.T) {
	ok := password.Verify("mysecret", "")
	if ok {
		t.Error("expected Verify to return false for empty hash")
	}
}

func TestValidate_Policy(t *testing.T) {
	ok := []string{"correct horse 9", "Tr0ub4dor&3x", "çãoáéíóú12"}
	for _, p := range ok {
		if err := password.Validate(p, "ana@exemplo.test", "ana"); err != nil {
			t.Errorf("%q: unexpected %v", p, err)
		}
	}
	bad := map[string]string{
		"short":           "abc123",
		"only letters":    "abcdefghijkl",
		"only digits":     "123456789012",
		"common":          "Password123",
		"contains e-mail": "mariana-2024x",
		"contains name":   "xx-mariana-9",
		"over 72 bytes":   strings.Repeat("a1", 37),
	}
	for name, p := range bad {
		t.Run(name, func(t *testing.T) {
			if err := password.Validate(p, "mariana@exemplo.test", "mariana"); !errors.Is(err, password.ErrWeak) {
				t.Errorf("%q: expected ErrWeak, got %v", p, err)
			}
		})
	}
}
