package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/password"
	"github.com/victorradael/condoguard/api/pkg/ratelimit"
	"github.com/victorradael/condoguard/api/pkg/secretbox"
	"github.com/victorradael/condoguard/api/pkg/totp"
)

// Errors of the second-factor flows.
var (
	// ErrEmailNotVerified is returned by Login when the password is right but the e-mail was never confirmed.
	ErrEmailNotVerified = errors.New("auth: e-mail not verified")
	// ErrInvalidMFAToken is returned for an expired, forged or mismatched step token.
	ErrInvalidMFAToken = errors.New("auth: invalid or expired two-factor token")
	// ErrInvalidCode is returned for a wrong, replayed or malformed code.
	ErrInvalidCode = errors.New("auth: invalid code")
	// ErrTooManyAttempts is returned when the codes of an account were tried too often.
	ErrTooManyAttempts = errors.New("auth: too many attempts")
	// ErrMFAAlreadyEnabled is returned by Setup when the account already has a second factor.
	ErrMFAAlreadyEnabled = errors.New("auth: two-factor authentication is already enabled")
	// ErrMFANotEnabled is returned when a step needs a second factor the account has not set up.
	ErrMFANotEnabled = errors.New("auth: two-factor authentication is not enabled")
	// ErrMFAMandatory is returned when someone whose role requires 2FA tries to turn it off.
	ErrMFAMandatory = errors.New("auth: two-factor authentication is mandatory for this role")
)

// StepTTL is how long the token between the password and the second factor lives.
const StepTTL = 5 * time.Minute

// RecoveryCodeCount is how many one-time recovery codes an account gets.
const RecoveryCodeCount = 10

// MaxCodeAttempts is how many wrong second-factor codes an account may try per window.
const MaxCodeAttempts = 5

// Security is the second-factor state of an account in the current condominium.
type Security struct {
	UserID        string
	Username      string
	Email         string
	EmailVerified bool
	SealedSecret  []byte // AES-GCM of the TOTP seed; nil when none
	TOTPEnabled   bool   // the seed is confirmed
	LastStep      int64  // last accepted 30-second step
}

// MFAStore persists the second factor. Every method runs in the condominium
// and for the user carried in ctx.
type MFAStore interface {
	// Security returns the state or ErrNotFound.
	Security(ctx context.Context, userID string) (*Security, error)
	// SetPending stores a new, unconfirmed seed. false when the account already has a confirmed one.
	SetPending(ctx context.Context, userID string, sealed []byte) (bool, error)
	// Enable confirms the pending seed and records the step of the code that proved it.
	Enable(ctx context.Context, userID string, step int64) (bool, error)
	// AdvanceStep records a used step; false when the step is not newer than the last one (a replay).
	AdvanceStep(ctx context.Context, userID string, step int64) (bool, error)
	// Clear removes the seed and the recovery codes.
	Clear(ctx context.Context, userID string) error
	// ReplaceRecoveryCodes drops the old codes and stores the hashes of the new ones.
	ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error
	// UseRecoveryCode spends a code; false when unknown or already used.
	UseRecoveryCode(ctx context.Context, userID string, hash []byte) (bool, error)
	// RecoveryCodesLeft counts the unused codes.
	RecoveryCodesLeft(ctx context.Context, userID string) (int, error)
}

// MFA is everything the service needs to run the second factor.
type MFA struct {
	Store   MFAStore
	Box     *secretbox.Box
	Limiter ratelimit.Limiter // attempts per account; nil = no limit
	Issuer  string            // name shown in the authenticator app
	Now     func() time.Time  // clock; nil = time.Now
}

func (m *MFA) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// Challenge is returned instead of tokens when the login needs a second step.
type Challenge struct {
	// Stage is "verify" (present a code) or "enroll" (set up the app first).
	Stage     string
	Token     string
	ExpiresIn time.Duration
}

// LoginResult holds either the tokens of a finished login or the challenge to answer.
type LoginResult struct {
	Tokens    *Tokens
	Challenge *Challenge
}

// Revocations is the list of access tokens that stopped working.
type revocations = middleware.Revocations

// WithMFA turns the second factor on.
func (s *Service) WithMFA(m *MFA) *Service {
	s.mfa = m
	return s
}

// WithRevocations makes logout and "sign out everywhere" revoke access tokens too.
func (s *Service) WithRevocations(r revocations) *Service {
	s.revoked = r
	return s
}

// WithSessionRevoker lets the service end every refresh token of an account (all condominiums).
func (s *Service) WithSessionRevoker(r SessionRevoker) *Service {
	s.sessions = r
	return s
}

// SessionRevoker ends the refresh tokens of an account.
type SessionRevoker interface {
	RevokeSessions(ctx context.Context, userID string) error
}

// stepContext puts the condominium and the user of a step token in ctx.
func stepContext(ctx context.Context, c *pkgjwt.Claims) context.Context {
	return tenant.WithUserID(tenant.WithCondominiumID(ctx, c.CondominiumID), c.UserID)
}

// challenge decides whether a finished password check still needs a second
// step. nil means the session can be opened.
func (s *Service) challenge(ctx context.Context, user *User, roles []string) (*Challenge, error) {
	if s.mfa == nil {
		return nil, nil
	}
	sec, err := s.mfa.Store.Security(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	var stage, purpose string
	switch {
	case sec.TOTPEnabled:
		stage, purpose = "verify", pkgjwt.PurposeMFAVerify
	case authz.RequiresMFA(roles):
		stage, purpose = "enroll", pkgjwt.PurposeMFAEnroll
	default:
		return nil, nil
	}
	tok, err := s.jwtSvc.GenerateStepToken(user.ID, user.CondominiumID, purpose, StepTTL)
	if err != nil {
		return nil, err
	}
	return &Challenge{Stage: stage, Token: tok, ExpiresIn: StepTTL}, nil
}

// StepIdentity validates a step token and returns the context to continue in.
func (s *Service) StepIdentity(ctx context.Context, token string, purposes ...string) (context.Context, *pkgjwt.Claims, error) {
	c, err := s.jwtSvc.ValidateStepToken(token, purposes...)
	if err != nil {
		return ctx, nil, ErrInvalidMFAToken
	}
	return stepContext(ctx, c), c, nil
}

// reserve counts one attempt against the account; the returned release gives
// it back (a code that was right does not count).
func (s *Service) reserve(ctx context.Context, userID string) (done func(success bool), err error) {
	if s.mfa.Limiter == nil {
		return func(bool) {}, nil
	}
	key := "mfa:" + userID
	ok, _, err := s.mfa.Limiter.Reserve(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTooManyAttempts
	}
	return func(success bool) {
		if success {
			_ = s.mfa.Limiter.Reset(context.WithoutCancel(ctx), key)
		}
	}, nil
}

// checkCode verifies a TOTP code or, if recovery is true, a recovery code. A
// TOTP code works once (its step must be newer than the last accepted one).
func (s *Service) checkCode(ctx context.Context, sec *Security, code string, recovery bool) error {
	if recovery {
		ok, err := s.mfa.Store.UseRecoveryCode(ctx, sec.UserID, hashRecoveryCode(code))
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidCode
		}
		return nil
	}
	if !sec.TOTPEnabled || sec.SealedSecret == nil {
		return ErrMFANotEnabled
	}
	seed, err := s.mfa.Box.Open(sec.SealedSecret, []byte(sec.UserID))
	if err != nil {
		return err
	}
	step, ok := totp.Verify(string(seed), code, s.mfa.now())
	if !ok {
		return ErrInvalidCode
	}
	fresh, err := s.mfa.Store.AdvanceStep(ctx, sec.UserID, step)
	if err != nil {
		return err
	}
	if !fresh {
		return ErrInvalidCode // the same code a second time
	}
	return nil
}

// VerifyRequest answers a "verify" challenge with a TOTP code or a recovery code.
type VerifyRequest struct {
	MFAToken     string `json:"mfaToken"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recoveryCode"`
	UseCookies   bool   `json:"useCookies"`
}

// VerifySecondFactor finishes a login that asked for a code.
func (s *Service) VerifySecondFactor(ctx context.Context, req VerifyRequest) (*Tokens, error) {
	if s.mfa == nil {
		return nil, ErrMFANotEnabled
	}
	ctx, claims, err := s.StepIdentity(ctx, req.MFAToken, pkgjwt.PurposeMFAVerify)
	if err != nil {
		return nil, err
	}
	if req.Code == "" && req.RecoveryCode == "" {
		return nil, fmt.Errorf("%w: code or recoveryCode is required", ErrValidation)
	}
	done, err := s.reserve(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	sec, err := s.mfa.Store.Security(ctx, claims.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidMFAToken // the membership is gone
	}
	if err != nil {
		return nil, err
	}
	if req.RecoveryCode != "" {
		err = s.checkCode(ctx, sec, req.RecoveryCode, true)
	} else {
		err = s.checkCode(ctx, sec, req.Code, false)
	}
	if err != nil {
		return nil, err
	}
	done(true)
	return s.openSession(ctx, claims.UserID)
}

// openSession issues the tokens of a user whose factors are all done.
func (s *Service) openSession(ctx context.Context, userID string) (*Tokens, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidMFAToken
	}
	if err != nil {
		return nil, err
	}
	return s.issue(ctx, user, newFamily())
}

// SetupResult is what the user puts into the authenticator app.
type SetupResult struct {
	Secret     string
	OTPAuthURL string
}

// SetupTwoFactor starts the enrolment: a new seed, pending until the user
// proves the app shows the right codes. The caller has already identified the
// user (access token or enrol step token) and put the condominium and user in ctx.
func (s *Service) SetupTwoFactor(ctx context.Context, userID string) (*SetupResult, error) {
	if s.mfa == nil {
		return nil, ErrMFANotEnabled
	}
	sec, err := s.mfa.Store.Security(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sec.TOTPEnabled {
		return nil, ErrMFAAlreadyEnabled
	}
	seed, err := totp.NewSecret()
	if err != nil {
		return nil, err
	}
	sealed, err := s.mfa.Box.Seal([]byte(seed), []byte(userID))
	if err != nil {
		return nil, err
	}
	ok, err := s.mfa.Store.SetPending(ctx, userID, sealed)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrMFAAlreadyEnabled
	}
	return &SetupResult{Secret: seed, OTPAuthURL: totp.URL(s.mfa.Issuer, sec.Email, seed)}, nil
}

// EnableTwoFactor confirms the enrolment with the first code and returns the
// recovery codes, shown once. With finish (an enrol step token) it also opens
// the session the login was waiting for.
func (s *Service) EnableTwoFactor(ctx context.Context, userID, code string, finish bool) (codes []string, tokens *Tokens, err error) {
	if s.mfa == nil {
		return nil, nil, ErrMFANotEnabled
	}
	done, err := s.reserve(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	sec, err := s.mfa.Store.Security(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if sec.TOTPEnabled {
		return nil, nil, ErrMFAAlreadyEnabled
	}
	if sec.SealedSecret == nil {
		return nil, nil, ErrMFANotEnabled // Setup first
	}
	seed, err := s.mfa.Box.Open(sec.SealedSecret, []byte(userID))
	if err != nil {
		return nil, nil, err
	}
	step, ok := totp.Verify(string(seed), code, s.mfa.now())
	if !ok {
		return nil, nil, ErrInvalidCode
	}
	if ok, err = s.mfa.Store.Enable(ctx, userID, step); err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrMFAAlreadyEnabled
	}
	if codes, err = s.newRecoveryCodes(ctx, userID); err != nil {
		return nil, nil, err
	}
	done(true)
	if finish {
		if tokens, err = s.openSession(ctx, userID); err != nil {
			return nil, nil, err
		}
	}
	return codes, tokens, nil
}

// RegenerateRecoveryCodes replaces the recovery codes; it asks for a current TOTP code.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, code string) ([]string, error) {
	if s.mfa == nil {
		return nil, ErrMFANotEnabled
	}
	done, err := s.reserve(ctx, userID)
	if err != nil {
		return nil, err
	}
	sec, err := s.mfa.Store.Security(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err = s.checkCode(ctx, sec, code, false); err != nil {
		return nil, err
	}
	codes, err := s.newRecoveryCodes(ctx, userID)
	if err != nil {
		return nil, err
	}
	done(true)
	return codes, nil
}

// DisableTwoFactor turns the second factor off for someone whose role does not
// require it. It asks for the password and a current code (or a recovery code).
func (s *Service) DisableTwoFactor(ctx context.Context, userID, pass, code string, recovery bool) error {
	if s.mfa == nil {
		return ErrMFANotEnabled
	}
	p, err := authz.FromContext(ctx)
	if err != nil {
		return err
	}
	if authz.RequiresMFA(p.Roles) {
		return ErrMFAMandatory
	}
	done, err := s.reserve(ctx, userID)
	if err != nil {
		return err
	}
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !password.Verify(pass, user.Password) {
		return ErrInvalidCredentials
	}
	sec, err := s.mfa.Store.Security(ctx, userID)
	if err != nil {
		return err
	}
	if !sec.TOTPEnabled {
		return ErrMFANotEnabled
	}
	if err = s.checkCode(ctx, sec, code, recovery); err != nil {
		return err
	}
	done(true)
	return s.mfa.Store.Clear(ctx, userID)
}

// ResetTwoFactor removes the second factor of another account of the
// condominium (a síndico helping someone who lost the phone and the codes) and
// ends that account's sessions. The caller has already been authorized; when
// the role requires 2FA, the person enrols again at the next login.
func (s *Service) ResetTwoFactor(ctx context.Context, userID string) error {
	if s.mfa == nil {
		return ErrMFANotEnabled
	}
	if err := s.mfa.Store.Clear(ctx, userID); err != nil {
		return err
	}
	return s.RevokeAllSessions(ctx, userID)
}

// RevokeAllSessions ends every session of the account: the refresh tokens (in
// every condominium) and the access tokens already issued.
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) error {
	if s.sessions != nil {
		if err := s.sessions.RevokeSessions(ctx, userID); err != nil {
			return err
		}
	}
	if s.revoked != nil {
		return s.revoked.RevokeUser(ctx, userID, time.Now(), s.jwtSvc.Expiry())
	}
	return nil
}

func (s *Service) newRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	codes := make([]string, RecoveryCodeCount)
	hashes := make([][]byte, RecoveryCodeCount)
	for i := range codes {
		c, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes[i], hashes[i] = c, hashRecoveryCode(c)
	}
	if err := s.mfa.Store.ReplaceRecoveryCodes(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

var codeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// newRecoveryCode returns 50 random bits as "ABCDE-FGHIJ".
func newRecoveryCode() (string, error) {
	b := make([]byte, 7)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := codeEncoding.EncodeToString(b)[:10]
	return raw[:5] + "-" + raw[5:], nil
}

func hashRecoveryCode(code string) []byte {
	canon := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code))
	sum := sha256.Sum256([]byte("recovery:" + canon))
	return sum[:]
}

func newFamily() string { return uuid.NewString() }
