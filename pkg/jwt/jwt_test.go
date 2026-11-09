package jwt_test

import (
	"errors"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/victorradael/condoguard/api/pkg/jwt"
)

const testSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi" // base64("test-secret-key-for-unit-testing-32b")

// ── NewService (secret strength) ─────────────────────────────────────────────

func TestJWT_NewService_RejectsMissingOrWeakSecret(t *testing.T) {
	for name, secret := range map[string]string{
		"empty":              "",
		"short raw":          "short-secret",
		"old placeholder":    "Y2hhbmdlLW1lLWluLXByb2R1Y3Rpb24=", // base64("change-me-in-production"), 23 bytes
		"31 bytes":           "0123456789012345678901234567890",
		"base64 of 16 bytes": "MDEyMzQ1Njc4OWFiY2RlZg==",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := jwt.NewService(secret); !errors.Is(err, jwt.ErrWeakSecret) {
				t.Fatalf("expected ErrWeakSecret, got %v", err)
			}
		})
	}
}

func TestJWT_NewService_AcceptsStrongSecret(t *testing.T) {
	for _, secret := range []string{testSecret, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="} {
		if _, err := jwt.NewService(secret); err != nil {
			t.Errorf("%q: %v", secret, err)
		}
	}
}

// ── GenerateToken ────────────────────────────────────────────────────────────

func TestJWT_GenerateToken_ReturnsNonEmptyString(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	token, err := svc.GenerateToken("user-123", "condo-a", []string{"ROLE_USER"})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestJWT_GenerateToken_DifferentUsersProduceDifferentTokens(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	token1, _ := svc.GenerateToken("user-1", "condo-a", []string{"ROLE_USER"})
	token2, _ := svc.GenerateToken("user-2", "condo-a", []string{"ROLE_USER"})

	if token1 == token2 {
		t.Error("expected different tokens for different users")
	}
}

func TestJWT_GenerateToken_EmptyUserOrCondominiumReturnsError(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	if _, err := svc.GenerateToken("", "condo-a", []string{"ROLE_USER"}); err == nil {
		t.Error("expected error for empty user ID")
	}
	if _, err := svc.GenerateToken("user-1", "", []string{"ROLE_USER"}); err == nil {
		t.Error("expected error for empty condominium ID")
	}
}

// ── ValidateToken ────────────────────────────────────────────────────────────

func TestJWT_ValidateToken_ValidTokenReturnsCorrectClaims(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	token, err := svc.GenerateToken("user-123", "condo-a", []string{"ROLE_ADMIN"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	if claims.UserID != "user-123" || claims.CondominiumID != "condo-a" {
		t.Errorf("unexpected claims: %+v", claims)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "ROLE_ADMIN" {
		t.Errorf("expected roles [ROLE_ADMIN], got %v", claims.Roles)
	}
	if claims.ID == "" || claims.Issuer != jwt.Issuer {
		t.Errorf("expected jti and issuer, got %+v", claims.RegisteredClaims)
	}
}

func TestJWT_ValidateToken_InvalidSignatureReturnsError(t *testing.T) {
	svc := jwt.MustNewService(testSecret)
	otherSvc := jwt.MustNewService("b3RoZXItc2VjcmV0LWtleS1mb3ItdGVzdGluZy0zMmJ5dGVz")

	token, _ := svc.GenerateToken("user-123", "condo-a", []string{"ROLE_USER"})

	if _, err := otherSvc.ValidateToken(token); err == nil {
		t.Fatal("expected error for token signed with different secret")
	}
}

// A token signed with the right key but a different algorithm, issuer or no
// expiry must be rejected.
func TestJWT_ValidateToken_RejectsForeignTokens(t *testing.T) {
	key, err := jwt.DecodeSecret(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	svc := jwt.MustNewService(testSecret)
	future := gojwt.NewNumericDate(time.Now().Add(time.Hour))

	sign := func(m gojwt.SigningMethod, c jwt.Claims) string {
		s, err := gojwt.NewWithClaims(m, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	base := func() jwt.Claims {
		return jwt.Claims{UserID: "u", CondominiumID: "c", RegisteredClaims: gojwt.RegisteredClaims{Issuer: jwt.Issuer, ExpiresAt: future}}
	}

	hs512 := sign(gojwt.SigningMethodHS512, base())
	noExp := base()
	noExp.ExpiresAt = nil
	otherIss := base()
	otherIss.Issuer = "someone-else"
	noCondo := base()
	noCondo.CondominiumID = ""
	none, _ := gojwt.NewWithClaims(gojwt.SigningMethodNone, base()).SignedString(gojwt.UnsafeAllowNoneSignatureType)

	for name, tok := range map[string]string{
		"HS512":          hs512,
		"alg none":       none,
		"no expiry":      sign(gojwt.SigningMethodHS256, noExp),
		"other issuer":   sign(gojwt.SigningMethodHS256, otherIss),
		"no condominium": sign(gojwt.SigningMethodHS256, noCondo),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.ValidateToken(tok); err == nil {
				t.Fatal("expected the token to be rejected")
			}
		})
	}
}

func TestJWT_ValidateToken_MalformedTokenReturnsError(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	if _, err := svc.ValidateToken("not.a.valid.jwt"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestJWT_ValidateToken_EmptyTokenReturnsError(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	if _, err := svc.ValidateToken(""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestJWT_ValidateToken_ExpiredTokenReturnsError(t *testing.T) {
	svc, err := jwt.NewServiceWithExpiry(testSecret, -1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	token, err := svc.GenerateToken("user-123", "condo-a", []string{"ROLE_USER"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err = svc.ValidateToken(token); err == nil {
		t.Fatal("expected error for expired token")
	}
}

// ── ExtractUserID ─────────────────────────────────────────────────────────────

func TestJWT_ExtractUserID_ValidToken(t *testing.T) {
	svc := jwt.MustNewService(testSecret)

	token, _ := svc.GenerateToken("user-abc", "condo-a", []string{"ROLE_USER"})
	userID, err := svc.ExtractUserID(token)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if userID != "user-abc" {
		t.Errorf("expected 'user-abc', got %q", userID)
	}
}
