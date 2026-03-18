package jwt

import (
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultExpiry = 10 * time.Hour

// Claims are the custom JWT claims used by CondoGuard.
type Claims struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// Service handles JWT generation and validation.
type Service struct {
	secret []byte
	expiry time.Duration
}

// NewService creates a Service using a Base64-encoded secret and the default
// 10-hour expiry from the original Java implementation.
func NewService(base64Secret string) *Service {
	return NewServiceWithExpiry(base64Secret, defaultExpiry)
}

// NewServiceWithExpiry creates a Service with a custom expiry duration.
// Negative durations produce tokens that are immediately expired (useful for tests).
func NewServiceWithExpiry(base64Secret string, expiry time.Duration) *Service {
	secret, err := base64.StdEncoding.DecodeString(base64Secret)
	if err != nil {
		// Fall back to raw bytes if the secret is not Base64-encoded.
		secret = []byte(base64Secret)
	}
	return &Service{secret: secret, expiry: expiry}
}

// GenerateToken creates a signed JWT for the given user.
func (s *Service) GenerateToken(userID string, roles []string) (string, error) {
	if userID == "" {
		return "", errors.New("jwt: userID must not be empty")
	}

	now := time.Now()
	claims := Claims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken parses and validates a JWT string, returning the claims on success.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("jwt: token must not be empty")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("jwt: unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("jwt: invalid token claims")
	}
	return claims, nil
}

// ExtractUserID validates the token and returns only the user ID.
func (s *Service) ExtractUserID(tokenString string) (string, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}
	return claims.UserID, nil
}
