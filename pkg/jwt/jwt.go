package jwt

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func init() {
	// Millisecond timestamps: a token issued right after a revocation must not
	// look older than it (seconds would round it into the revoked second).
	jwt.TimePrecision = time.Millisecond
}

// DefaultExpiry is the access-token lifetime. It is short on purpose: a
// stolen access token cannot be revoked, so it must expire quickly. Sessions
// are kept alive with the refresh token (see internal/auth).
const DefaultExpiry = 15 * time.Minute

// MinSecretBytes is the smallest accepted HMAC key (256 bits, the HS256 output size).
const MinSecretBytes = 32

// Issuer is written to and required in every token.
const Issuer = "condoguard-api"

// ErrWeakSecret is returned when the signing secret is missing or too short.
var ErrWeakSecret = fmt.Errorf("jwt: secret must be Base64 of at least %d bytes (openssl rand -base64 32)", MinSecretBytes)

// Claims are the custom JWT claims used by CondoGuard.
type Claims struct {
	UserID        string   `json:"user_id"`
	CondominiumID string   `json:"cid"`
	Roles         []string `json:"roles"`
	// Purpose is empty in access tokens. Short-lived tokens that only unlock one
	// step (the second factor) carry it, and ValidateToken refuses them: a
	// half-finished login can never act as a session.
	Purpose string `json:"pur,omitempty"`
	jwt.RegisteredClaims
}

// Service handles JWT generation and validation.
type Service struct {
	secret []byte
	expiry time.Duration
}

// NewService creates a Service with the default access-token expiry.
// The secret must be Base64 (standard encoding) of at least MinSecretBytes
// random bytes, e.g. `openssl rand -base64 32`; otherwise ErrWeakSecret is returned.
func NewService(secret string) (*Service, error) {
	return NewServiceWithExpiry(secret, DefaultExpiry)
}

// NewServiceWithExpiry creates a Service with a custom expiry duration.
// Negative durations produce tokens that are immediately expired (useful for tests).
func NewServiceWithExpiry(secret string, expiry time.Duration) (*Service, error) {
	key, err := DecodeSecret(secret)
	if err != nil {
		return nil, err
	}
	return &Service{secret: key, expiry: expiry}, nil
}

// DecodeSecret returns the HMAC key encoded in secret (standard Base64).
// Anything that is not Base64, or decodes to fewer than MinSecretBytes bytes,
// is rejected: a raw passphrase would be both ambiguous and weak.
func DecodeSecret(secret string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil || len(key) < MinSecretBytes {
		return nil, ErrWeakSecret
	}
	return key, nil
}

// Expiry returns the lifetime of the tokens this Service generates.
func (s *Service) Expiry() time.Duration { return s.expiry }

// GenerateToken creates a signed JWT for the given user of the given condominium.
func (s *Service) GenerateToken(userID, condominiumID string, roles []string) (string, error) {
	if userID == "" {
		return "", errors.New("jwt: userID must not be empty")
	}
	if condominiumID == "" {
		return "", errors.New("jwt: condominiumID must not be empty")
	}

	now := time.Now()
	claims := Claims{
		UserID:        userID,
		CondominiumID: condominiumID,
		Roles:         roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Issuer:    Issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// Purposes of the step tokens (see Claims.Purpose).
const (
	// PurposeMFAVerify is issued after the password when the account has a second factor to present.
	PurposeMFAVerify = "mfa_verify"
	// PurposeMFAEnroll is issued after the password when the role requires a second factor the account has not set up yet.
	PurposeMFAEnroll = "mfa_enroll"
)

// GenerateStepToken creates a token that is valid only for the given purpose
// and only for ttl. It carries no roles.
func (s *Service) GenerateStepToken(userID, condominiumID, purpose string, ttl time.Duration) (string, error) {
	if userID == "" || condominiumID == "" || purpose == "" {
		return "", errors.New("jwt: userID, condominiumID and purpose must not be empty")
	}
	now := time.Now()
	claims := Claims{
		UserID:        userID,
		CondominiumID: condominiumID,
		Purpose:       purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Issuer:    Issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// ValidateStepToken validates a token made by GenerateStepToken for one of the purposes.
func (s *Service) ValidateStepToken(tokenString string, purposes ...string) (*Claims, error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return nil, err
	}
	for _, p := range purposes {
		if claims.Purpose != "" && claims.Purpose == p {
			return claims, nil
		}
	}
	return nil, errors.New("jwt: token is not valid for this step")
}

// ValidateToken parses and validates a JWT string, returning the claims on success.
// Only HS256 tokens from this issuer, with an expiry, a user and a condominium are accepted,
// and step tokens (Claims.Purpose) are not.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Purpose != "" {
		return nil, errors.New("jwt: step token used as an access token")
	}
	return claims, nil
}

func (s *Service) parse(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("jwt: token must not be empty")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(*jwt.Token) (any, error) {
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("jwt: invalid token claims")
	}
	if claims.UserID == "" || claims.CondominiumID == "" {
		return nil, errors.New("jwt: token without user or condominium")
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

// MustNewService is NewService for tests and fixed, known-good secrets: it panics on error.
func MustNewService(secret string) *Service {
	s, err := NewService(secret)
	if err != nil {
		panic(err)
	}
	return s
}
