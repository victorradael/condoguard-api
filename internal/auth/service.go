package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgemail "github.com/victorradael/condoguard/api/pkg/email"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/password"
)

// ErrInvalidCredentials is returned when e-mail or password is wrong.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrInvalidRefreshToken is returned for an unknown, expired or revoked refresh token.
var ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")

// ErrRegistrationClosed is returned by sign-up when no default condominium is configured.
var ErrRegistrationClosed = errors.New("auth: sign-up is not open")

// CondominiumRequiredError is returned by Login when the account belongs to
// several condominiums and the request did not say which one to sign in to.
// It is only returned after the password has been checked.
type CondominiumRequiredError struct {
	Condominiums []Membership
}

func (*CondominiumRequiredError) Error() string { return "auth: condominiumId is required" }

// ErrValidation signals an input validation failure.
var ErrValidation = errors.New("validation error")

// DefaultRefreshTTL is the refresh-token lifetime. Every refresh issues a new
// token with a new lifetime, so an active session stays open; an idle one
// ends after this long.
const DefaultRefreshTTL = 30 * 24 * time.Hour

// Tokens is the outcome of a login or refresh.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	Roles        []string
}

// Pair returns the JSON body for the token (app) flow.
func (t *Tokens) Pair() TokenPair {
	return TokenPair{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, TokenType: "Bearer",
		ExpiresIn: int(t.AccessTTL.Seconds()), Roles: t.Roles}
}

// Service holds the business logic for the auth domain.
type Service struct {
	repo       Repository
	tokens     TokenStore
	jwtSvc     *pkgjwt.Service
	refreshTTL time.Duration
	mfa        *MFA               // nil = no second factor (tests, tools)
	revoked    revocations        // nil = access tokens are not revocable
	sessions   SessionRevoker     // nil = only the family of the logout is revoked
	verified   bool               // require a confirmed e-mail to sign in
	notify     VerificationSender // e-mails the verification link after sign-up
}

// VerificationSender e-mails a freshly created account its verification link.
type VerificationSender interface {
	SendVerification(ctx context.Context, userID, email, username string)
}

// WithVerificationMail makes sign-up send the verification link.
func (s *Service) WithVerificationMail(n VerificationSender) *Service {
	s.notify = n
	return s
}

// NewService creates an auth Service. refreshTTL <= 0 means DefaultRefreshTTL.
func NewService(repo Repository, tokens TokenStore, jwtSvc *pkgjwt.Service, refreshTTL time.Duration) *Service {
	if refreshTTL <= 0 {
		refreshTTL = DefaultRefreshTTL
	}
	return &Service{repo: repo, tokens: tokens, jwtSvc: jwtSvc, refreshTTL: refreshTTL}
}

// Register creates a resident account (morador) in the default condominium
// after validating the request and hashing the password. Whatever the client
// sends, sign-up never grants another role.
func (s *Service) Register(ctx context.Context, req RegisterRequest) error {
	if _, err := tenant.CondominiumID(ctx); err != nil {
		return ErrRegistrationClosed
	}
	email, err := pkgemail.Normalize(req.Email)
	if err != nil {
		return fmt.Errorf("%w: a valid email is required", ErrValidation)
	}
	if req.Username == "" {
		return fmt.Errorf("%w: username is required", ErrValidation)
	}
	if err = password.Validate(req.Password, localPart(email), req.Username); err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}

	hashed, err := password.Hash(req.Password)
	if err != nil {
		return err
	}

	user := &User{
		Username: req.Username,
		Email:    email,
		Password: hashed,
		Roles:    []string{authz.RoleMorador},
	}
	if err = s.repo.Save(ctx, user); err != nil {
		return err
	}
	if s.notify != nil {
		s.notify.SendVerification(ctx, user.ID, user.Email, user.Username)
	}
	return nil
}

// RequireVerifiedEmail makes Login refuse accounts whose e-mail was never confirmed.
func (s *Service) RequireVerifiedEmail() *Service {
	s.verified = true
	return s
}

// Login validates credentials and, when nothing else is asked, opens a
// session (access + refresh token) in one condominium: the requested one, or
// the only one of the account. When the account has a second factor, or its
// role requires one, the result is a Challenge instead of tokens.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResult, error) {
	if req.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrValidation)
	}
	if req.Password == "" {
		return nil, fmt.Errorf("%w: password is required", ErrValidation)
	}

	email, err := pkgemail.Normalize(req.Email)
	if err != nil {
		equaliseTiming(req.Password)
		return nil, ErrInvalidCredentials
	}
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Same bcrypt cost as a wrong password: response time must not
			// tell which e-mails have an account.
			equaliseTiming(req.Password)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if !password.Verify(req.Password, user.Password) {
		return nil, ErrInvalidCredentials
	}

	if s.verified && !user.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	member, err := s.membershipFor(ctx, user.ID, req.CondominiumID)
	if err != nil {
		return nil, err
	}
	user.Roles, user.CondominiumID = member.Roles, member.CondominiumID
	ctx = tenant.WithUserID(tenant.WithCondominiumID(ctx, member.CondominiumID), user.ID)

	ch, err := s.challenge(ctx, user, member.Roles)
	if err != nil {
		return nil, err
	}
	if ch != nil {
		return &LoginResult{Challenge: ch}, nil
	}
	t, err := s.issue(ctx, user, uuid.NewString())
	if err != nil {
		return nil, err
	}
	return &LoginResult{Tokens: t}, nil
}

// membershipFor picks the condominium of a login. A condominium the account
// does not belong to looks like a wrong password.
func (s *Service) membershipFor(ctx context.Context, userID, wanted string) (Membership, error) {
	all, err := s.repo.Memberships(ctx, userID)
	if err != nil {
		return Membership{}, err
	}
	if wanted != "" {
		for _, m := range all {
			if strings.EqualFold(m.CondominiumID, wanted) {
				return m, nil
			}
		}
		return Membership{}, ErrInvalidCredentials
	}
	switch len(all) {
	case 0:
		return Membership{}, ErrInvalidCredentials
	case 1:
		return all[0], nil
	}
	return Membership{}, &CondominiumRequiredError{Condominiums: all}
}

// Refresh exchanges a valid refresh token for a new pair (rotation). The old
// token stops working. Presenting a token that was already exchanged means it
// leaked: the whole session is revoked and the call fails.
func (s *Service) Refresh(ctx context.Context, raw string) (*Tokens, error) {
	if raw == "" {
		return nil, ErrInvalidRefreshToken
	}
	hash := hashToken(raw)
	ctx, err := s.tokenTenant(ctx, hash)
	if err != nil {
		return nil, err
	}
	sess, err := s.tokens.Consume(ctx, hash)
	if errors.Is(err, ErrTokenNotFound) {
		if family, ferr := s.tokens.Family(ctx, hash); ferr == nil {
			if rerr := s.tokens.RevokeFamily(ctx, family); rerr != nil {
				return nil, rerr
			}
		}
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindByID(ctx, sess.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}
	return s.issue(ctx, user, sess.FamilyID)
}

// Logout revokes the session the refresh token belongs to and, when it is
// given and valid, the access token too. Unknown tokens are ignored: logging
// out is always successful for the caller.
func (s *Service) Logout(ctx context.Context, raw, accessToken string) error {
	if accessToken != "" && s.revoked != nil {
		if c, err := s.jwtSvc.ValidateToken(accessToken); err == nil && c.ExpiresAt != nil {
			if err := s.revoked.Revoke(ctx, c.ID, c.ExpiresAt.Time); err != nil {
				return err
			}
		}
	}
	if raw == "" {
		return nil
	}
	hash := hashToken(raw)
	ctx, err := s.tokenTenant(ctx, hash)
	if errors.Is(err, ErrInvalidRefreshToken) {
		return nil
	}
	if err != nil {
		return err
	}
	family, err := s.tokens.Family(ctx, hash)
	if errors.Is(err, ErrTokenNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.tokens.RevokeFamily(ctx, family)
}

// tokenTenant puts the condominium that issued the refresh token in ctx: the
// refresh and logout requests carry no other hint of it.
func (s *Service) tokenTenant(ctx context.Context, hash []byte) (context.Context, error) {
	condo, err := s.tokens.CondominiumOf(ctx, hash)
	if errors.Is(err, ErrTokenNotFound) {
		return ctx, ErrInvalidRefreshToken
	}
	if err != nil {
		return ctx, err
	}
	return tenant.WithCondominiumID(ctx, condo), nil
}

func (s *Service) issue(ctx context.Context, user *User, familyID string) (*Tokens, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	access, err := s.jwtSvc.GenerateToken(user.ID, condoID, user.Roles)
	if err != nil {
		return nil, err
	}
	refresh, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	sess := Session{UserID: user.ID, FamilyID: familyID}
	if err := s.tokens.Insert(ctx, sess, hashToken(refresh), time.Now().Add(s.refreshTTL)); err != nil {
		return nil, err
	}
	return &Tokens{AccessToken: access, RefreshToken: refresh, AccessTTL: s.jwtSvc.Expiry(),
		RefreshTTL: s.refreshTTL, Roles: user.Roles}, nil
}

// newRefreshToken returns 256 random bits, URL-safe.
func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// equaliseTiming spends the same bcrypt work as checking a real password.
func equaliseTiming(plain string) {
	dummyHashOnce.Do(func() { dummyHash, _ = password.Hash("not-a-real-password-0") })
	_ = password.Verify(plain, dummyHash)
}

func localPart(email string) string {
	for i := range len(email) {
		if email[i] == '@' {
			return email[:i]
		}
	}
	return email
}
