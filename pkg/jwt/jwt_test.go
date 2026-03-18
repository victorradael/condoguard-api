package jwt_test

import (
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/pkg/jwt"
)

const testSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc=" // base64("test-secret-key-for-unit-testing")

// ── GenerateToken ────────────────────────────────────────────────────────────

func TestJWT_GenerateToken_ReturnsNonEmptyString(t *testing.T) {
	svc := jwt.NewService(testSecret)

	token, err := svc.GenerateToken("user-123", []string{"ROLE_USER"})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestJWT_GenerateToken_DifferentUsersProduceDifferentTokens(t *testing.T) {
	svc := jwt.NewService(testSecret)

	token1, _ := svc.GenerateToken("user-1", []string{"ROLE_USER"})
	token2, _ := svc.GenerateToken("user-2", []string{"ROLE_USER"})

	if token1 == token2 {
		t.Error("expected different tokens for different users")
	}
}

func TestJWT_GenerateToken_EmptyUserIDReturnsError(t *testing.T) {
	svc := jwt.NewService(testSecret)

	_, err := svc.GenerateToken("", []string{"ROLE_USER"})

	if err == nil {
		t.Fatal("expected error for empty user ID, got nil")
	}
}

// ── ValidateToken ────────────────────────────────────────────────────────────

func TestJWT_ValidateToken_ValidTokenReturnsCorrectClaims(t *testing.T) {
	svc := jwt.NewService(testSecret)

	token, err := svc.GenerateToken("user-123", []string{"ROLE_ADMIN"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if claims.UserID != "user-123" {
		t.Errorf("expected UserID 'user-123', got %q", claims.UserID)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "ROLE_ADMIN" {
		t.Errorf("expected roles [ROLE_ADMIN], got %v", claims.Roles)
	}
}

func TestJWT_ValidateToken_InvalidSignatureReturnsError(t *testing.T) {
	svc := jwt.NewService(testSecret)
	otherSvc := jwt.NewService("b3RoZXItc2VjcmV0LWtleS1mb3ItdGVzdGluZw==")

	token, _ := svc.GenerateToken("user-123", []string{"ROLE_USER"})

	_, err := otherSvc.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for token signed with different secret")
	}
}

func TestJWT_ValidateToken_MalformedTokenReturnsError(t *testing.T) {
	svc := jwt.NewService(testSecret)

	_, err := svc.ValidateToken("not.a.valid.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestJWT_ValidateToken_EmptyTokenReturnsError(t *testing.T) {
	svc := jwt.NewService(testSecret)

	_, err := svc.ValidateToken("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestJWT_ValidateToken_ExpiredTokenReturnsError(t *testing.T) {
	svc := jwt.NewServiceWithExpiry(testSecret, -1*time.Hour)

	token, err := svc.GenerateToken("user-123", []string{"ROLE_USER"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	_, err = svc.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

// ── ExtractUserID ─────────────────────────────────────────────────────────────

func TestJWT_ExtractUserID_ValidToken(t *testing.T) {
	svc := jwt.NewService(testSecret)

	token, _ := svc.GenerateToken("user-abc", []string{"ROLE_USER"})
	userID, err := svc.ExtractUserID(token)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if userID != "user-abc" {
		t.Errorf("expected 'user-abc', got %q", userID)
	}
}
