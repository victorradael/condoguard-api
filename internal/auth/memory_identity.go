package auth

import (
	"bytes"
	"context"
	"time"

	"github.com/victorradael/condoguard/api/internal/account"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

type memSecurity struct {
	sealed   []byte
	enabled  bool
	lastStep int64
	codes    map[string]bool // hash → used
}

type memAccountToken struct {
	userID, purpose string
	expires         time.Time
	used            bool
}

// ── account.Store ─────────────────────────────────────────────────────────────

var _ account.Store = memAccountStore{}
var _ MFAStore = (*MemoryIdentity)(nil)

func (r *MemoryIdentity) secOf(userID string) *memSecurity {
	s := r.sec[userID]
	if s == nil {
		s = &memSecurity{codes: map[string]bool{}}
		r.sec[userID] = s
	}
	return s
}

// FindByEmailAccount is the account.Store lookup by e-mail.
func (r *MemoryIdentity) FindByEmailAccount(_ context.Context, email string) (account.Account, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.store {
		if u.Email == email {
			return account.Account{ID: u.ID, Email: u.Email, Username: u.Username, Verified: u.EmailVerified}, true, nil
		}
	}
	return account.Account{}, false, nil
}

// AccountStore returns the account.Store view (its FindByEmail differs from Repository's).
func (r *MemoryIdentity) AccountStore() account.Store { return memAccountStore{r} }

type memAccountStore struct{ r *MemoryIdentity }

func (m memAccountStore) FindByEmail(ctx context.Context, email string) (account.Account, bool, error) {
	return m.r.FindByEmailAccount(ctx, email)
}

func (m memAccountStore) Issue(_ context.Context, userID, purpose string, hash []byte, expires time.Time) error {
	m.r.mu.Lock()
	defer m.r.mu.Unlock()
	m.r.supersede(userID, purpose)
	m.r.tokens[string(hash)] = &memAccountToken{userID: userID, purpose: purpose, expires: expires}
	return nil
}

func (m memAccountStore) IssueForMember(ctx context.Context, userID, purpose string, hash []byte, expires time.Time, _ string) (bool, error) {
	condo, err := tenant.CondominiumID(ctx)
	if err != nil {
		return false, nil
	}
	m.r.mu.Lock()
	defer m.r.mu.Unlock()
	if _, member := m.r.memberships[userID][condo]; !member {
		return false, nil
	}
	m.r.supersede(userID, purpose)
	m.r.tokens[string(hash)] = &memAccountToken{userID: userID, purpose: purpose, expires: expires}
	return true, nil
}

// supersede invalidates the unused tokens of the same kind. Caller holds the lock.
func (r *MemoryIdentity) supersede(userID, purpose string) {
	for _, t := range r.tokens {
		if t.userID == userID && t.purpose == purpose && !t.used {
			t.used = true
		}
	}
}

func (m memAccountStore) live(hash []byte, purpose string) *memAccountToken {
	t := m.r.tokens[string(hash)]
	if t == nil || t.purpose != purpose || t.used || !time.Now().Before(t.expires) {
		return nil
	}
	return t
}

func (m memAccountStore) Peek(_ context.Context, hash []byte, purpose string) (account.Peek, error) {
	m.r.mu.RLock()
	defer m.r.mu.RUnlock()
	t := m.live(hash, purpose)
	if t == nil {
		return account.Peek{}, account.ErrInvalidToken
	}
	u := m.r.store[t.userID]
	return account.Peek{UserID: u.ID, Email: u.Email, Username: u.Username}, nil
}

func (m memAccountStore) VerifyEmail(_ context.Context, hash []byte) (string, error) {
	m.r.mu.Lock()
	defer m.r.mu.Unlock()
	t := m.live(hash, account.PurposeVerify)
	if t == nil {
		return "", account.ErrInvalidToken
	}
	t.used = true
	m.r.store[t.userID].EmailVerified = true
	return t.userID, nil
}

func (m memAccountStore) ResetPassword(ctx context.Context, hash []byte, passwordHash string) (string, error) {
	m.r.mu.Lock()
	t := m.live(hash, account.PurposeReset)
	if t == nil {
		m.r.mu.Unlock()
		return "", account.ErrInvalidToken
	}
	t.used = true
	u := m.r.store[t.userID]
	u.Password, u.EmailVerified = passwordHash, true
	sessions := m.r.sessions
	m.r.mu.Unlock()
	if sessions != nil {
		if err := sessions.RevokeSessions(ctx, u.ID); err != nil {
			return "", err
		}
	}
	return u.ID, nil
}

func (m memAccountStore) RevokeSessions(ctx context.Context, userID string) error {
	m.r.mu.RLock()
	sessions := m.r.sessions
	m.r.mu.RUnlock()
	if sessions == nil {
		return nil
	}
	return sessions.RevokeSessions(ctx, userID)
}

// RevokeThrough makes password resets and revocations end the refresh tokens of tokens.
func (r *MemoryIdentity) RevokeThrough(s SessionRevoker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = s
}

// RevokeSessions ends the refresh tokens of the account in every condominium.
func (s *inMemoryTokenStore) RevokeSessions(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.session.UserID == userID {
			t.revoked = true
		}
	}
	return nil
}

// ── MFAStore ──────────────────────────────────────────────────────────────────

func (r *MemoryIdentity) member(ctx context.Context, userID string) (*User, error) {
	condo, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	u, ok := r.store[userID]
	if _, member := r.memberships[userID][condo]; !ok || !member {
		return nil, ErrNotFound
	}
	return u, nil
}

// Security implements MFAStore.
func (r *MemoryIdentity) Security(ctx context.Context, userID string) (*Security, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, err := r.member(ctx, userID)
	if err != nil {
		return nil, err
	}
	s := r.secOf(userID)
	return &Security{UserID: u.ID, Username: u.Username, Email: u.Email, EmailVerified: u.EmailVerified,
		SealedSecret: bytes.Clone(s.sealed), TOTPEnabled: s.enabled, LastStep: s.lastStep}, nil
}

// SetPending implements MFAStore.
func (r *MemoryIdentity) SetPending(ctx context.Context, userID string, sealed []byte) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return false, err
	}
	s := r.secOf(userID)
	if s.enabled {
		return false, nil
	}
	s.sealed, s.lastStep = bytes.Clone(sealed), 0
	return true, nil
}

// Enable implements MFAStore.
func (r *MemoryIdentity) Enable(ctx context.Context, userID string, step int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return false, err
	}
	s := r.secOf(userID)
	if s.enabled || s.sealed == nil {
		return false, nil
	}
	s.enabled, s.lastStep = true, step
	return true, nil
}

// AdvanceStep implements MFAStore.
func (r *MemoryIdentity) AdvanceStep(ctx context.Context, userID string, step int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return false, err
	}
	s := r.secOf(userID)
	if !s.enabled || s.lastStep >= step {
		return false, nil
	}
	s.lastStep = step
	return true, nil
}

// Clear implements MFAStore.
func (r *MemoryIdentity) Clear(ctx context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return err
	}
	delete(r.sec, userID)
	return nil
}

// ReplaceRecoveryCodes implements MFAStore.
func (r *MemoryIdentity) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return err
	}
	s := r.secOf(userID)
	s.codes = map[string]bool{}
	for _, h := range hashes {
		s.codes[string(h)] = false
	}
	return nil
}

// UseRecoveryCode implements MFAStore.
func (r *MemoryIdentity) UseRecoveryCode(ctx context.Context, userID string, hash []byte) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return false, err
	}
	s := r.secOf(userID)
	used, ok := s.codes[string(hash)]
	if !ok || used {
		return false, nil
	}
	s.codes[string(hash)] = true
	return true, nil
}

// RecoveryCodesLeft implements MFAStore.
func (r *MemoryIdentity) RecoveryCodesLeft(ctx context.Context, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.member(ctx, userID); err != nil {
		return 0, err
	}
	n := 0
	for _, used := range r.secOf(userID).codes {
		if !used {
			n++
		}
	}
	return n, nil
}
