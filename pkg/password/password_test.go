package password_test

import (
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
