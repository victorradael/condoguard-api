package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/tenant"
)

// ErrNotFound is returned when a user cannot be found.
var ErrNotFound = errors.New("auth: user not found")

// ErrDuplicateEmail is returned when the e-mail already exists.
var ErrDuplicateEmail = errors.New("auth: email already registered")

// ErrTokenNotFound is returned when a refresh token is unknown.
var ErrTokenNotFound = errors.New("auth: refresh token not found")

// Repository defines the persistence contract for the auth domain.
// Interfaces are defined on the consumer side (idiomatic Go).
// E-mails are passed already normalised (see pkg/email).
//
// An account is global (one e-mail, one account); its roles belong to its
// membership in each condominium.
type Repository interface {
	// Save creates the account with a membership in the condominium of ctx,
	// with user.Roles, in one step.
	Save(ctx context.Context, user *User) error
	// FindByEmail finds the account in the whole base (login happens before the
	// condominium is known). The roles are not filled in: see Memberships.
	FindByEmail(ctx context.Context, email string) (*User, error)
	// Memberships lists the condominiums the account belongs to.
	Memberships(ctx context.Context, userID string) ([]Membership, error)
	// FindByID finds the account through its membership in the condominium of
	// ctx; the roles are the ones it holds there.
	FindByID(ctx context.Context, id string) (*User, error)
}

// Session identifies the owner of a refresh token.
type Session struct {
	UserID   string
	FamilyID string
}

// TokenStore persists refresh tokens by their SHA-256 hash.
type TokenStore interface {
	// Insert stores a new, valid token of the given session.
	Insert(ctx context.Context, s Session, hash []byte, expiresAt time.Time) error
	// Consume revokes a valid token and returns its session in one atomic
	// step. Unknown, expired and already revoked tokens give ErrTokenNotFound.
	Consume(ctx context.Context, hash []byte) (Session, error)
	// Family returns the session family of any known token, valid or not.
	Family(ctx context.Context, hash []byte) (string, error)
	// RevokeFamily revokes every token of the session family.
	RevokeFamily(ctx context.Context, familyID string) error
	// CondominiumOf says which condominium issued the token; the refresh
	// request itself carries no tenant. ErrTokenNotFound when unknown.
	CondominiumOf(ctx context.Context, hash []byte) (string, error)
}

// ── In-memory (unit / integration without DB) ─────────────────────────────────

// MemoryIdentity is the in-memory stand-in for the whole identity side of the
// database: accounts and memberships (Repository), account tokens
// (account.Store) and the second factor (MFAStore). Dev mode and tests only.
type MemoryIdentity struct {
	mu          sync.RWMutex
	store       map[string]*User               // keyed by ID
	memberships map[string]map[string][]string // user ID → condominium ID → roles
	sec         map[string]*memSecurity        // user ID → second factor
	tokens      map[string]*memAccountToken    // token hash → token
	sessions    SessionRevoker                 // refresh-token store to revoke through, optional
}

// NewMemoryIdentity returns an empty MemoryIdentity.
func NewMemoryIdentity() *MemoryIdentity {
	return &MemoryIdentity{
		store: make(map[string]*User), memberships: make(map[string]map[string][]string),
		sec: make(map[string]*memSecurity), tokens: make(map[string]*memAccountToken),
	}
}

// NewInMemoryRepository returns a thread-safe in-memory Repository.
// Like the database, e-mails are unique across all condominiums.
func NewInMemoryRepository() Repository { return NewMemoryIdentity() }

func (r *MemoryIdentity) Save(ctx context.Context, user *User) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.store {
		if u.Email == user.Email {
			return ErrDuplicateEmail
		}
	}

	user.ID = uuid.NewString()
	user.CondominiumID = condoID
	cp := *user
	r.store[user.ID] = &cp
	r.memberships[user.ID] = map[string][]string{condoID: slices.Clone(user.Roles)}
	return nil
}

func (r *MemoryIdentity) FindByEmail(_ context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.store {
		if u.Email == email {
			cp := *u
			cp.Roles, cp.CondominiumID = nil, ""
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *MemoryIdentity) Memberships(_ context.Context, userID string) ([]Membership, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Membership, 0, len(r.memberships[userID]))
	for condo, roles := range r.memberships[userID] {
		out = append(out, Membership{CondominiumID: condo, Name: condo, Roles: slices.Clone(roles)})
	}
	slices.SortFunc(out, func(a, b Membership) int { return strings.Compare(a.CondominiumID, b.CondominiumID) })
	return out, nil
}

func (r *MemoryIdentity) FindByID(ctx context.Context, id string) (*User, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.store[id]
	roles, member := r.memberships[id][condoID]
	if !ok || !member {
		return nil, ErrNotFound
	}
	cp := *u
	cp.CondominiumID, cp.Roles = condoID, slices.Clone(roles)
	return &cp, nil
}

type memToken struct {
	condoID   string
	session   Session
	expiresAt time.Time
	revoked   bool
}

type inMemoryTokenStore struct {
	mu     sync.Mutex
	tokens map[string]*memToken // keyed by hash
}

// NewInMemoryTokenStore returns a thread-safe in-memory TokenStore.
func NewInMemoryTokenStore() TokenStore {
	return &inMemoryTokenStore{tokens: make(map[string]*memToken)}
}

func (s *inMemoryTokenStore) Insert(ctx context.Context, sess Session, hash []byte, expiresAt time.Time) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[string(hash)] = &memToken{condoID: condoID, session: sess, expiresAt: expiresAt}
	return nil
}

func (s *inMemoryTokenStore) lookup(ctx context.Context, hash []byte) (*memToken, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := s.tokens[string(hash)]
	if !ok || t.condoID != condoID {
		return nil, ErrTokenNotFound
	}
	return t, nil
}

func (s *inMemoryTokenStore) Consume(ctx context.Context, hash []byte) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.lookup(ctx, hash)
	if err != nil {
		return Session{}, err
	}
	if t.revoked || !time.Now().Before(t.expiresAt) {
		return Session{}, ErrTokenNotFound
	}
	t.revoked = true
	return t.session, nil
}

func (s *inMemoryTokenStore) Family(ctx context.Context, hash []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.lookup(ctx, hash)
	if err != nil {
		return "", err
	}
	return t.session.FamilyID, nil
}

func (s *inMemoryTokenStore) CondominiumOf(_ context.Context, hash []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[string(hash)]
	if !ok {
		return "", ErrTokenNotFound
	}
	return t.condoID, nil
}

func (s *inMemoryTokenStore) RevokeFamily(ctx context.Context, familyID string) error {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.condoID == condoID && t.session.FamilyID == familyID {
			t.revoked = true
		}
	}
	return nil
}
