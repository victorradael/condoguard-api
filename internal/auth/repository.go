package auth

import (
	"context"
	"errors"
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
type Repository interface {
	Save(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
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
}

// ── In-memory (unit / integration without DB) ─────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*User // keyed by ID
}

// NewInMemoryRepository returns a thread-safe in-memory Repository.
// Like the database, e-mails are unique across all condominiums.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*User)}
}

func (r *inMemoryRepository) Save(ctx context.Context, user *User) error {
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
	return nil
}

func (r *inMemoryRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.find(ctx, func(u *User) bool { return u.Email == email })
}

func (r *inMemoryRepository) FindByID(ctx context.Context, id string) (*User, error) {
	return r.find(ctx, func(u *User) bool { return u.ID == id })
}

func (r *inMemoryRepository) find(ctx context.Context, match func(*User) bool) (*User, error) {
	condoID, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.store {
		if u.CondominiumID == condoID && match(u) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, ErrNotFound
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
