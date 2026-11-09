package auth

import (
	"context"
	"errors"
	"sync"
)

// ErrNotFound is returned when a user cannot be found.
var ErrNotFound = errors.New("auth: user not found")

// ErrDuplicateEmail is returned when the e-mail already exists.
var ErrDuplicateEmail = errors.New("auth: email already registered")

// Repository defines the persistence contract for the auth domain.
// Interfaces are defined on the consumer side (idiomatic Go).
type Repository interface {
	Save(ctx context.Context, user *User) error
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
}

// ── In-memory (unit / integration without DB) ─────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*User // keyed by username
}

// NewInMemoryRepository returns a thread-safe in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*User)}
}

func (r *inMemoryRepository) Save(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.store {
		if u.Email == user.Email {
			return ErrDuplicateEmail
		}
	}

	if user.ID == "" {
		user.ID = user.Username // simple deterministic ID for tests
	}
	r.store[user.Username] = user
	return nil
}

func (r *inMemoryRepository) FindByUsername(_ context.Context, username string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.store[username]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (r *inMemoryRepository) FindByEmail(_ context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.store {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, ErrNotFound
}
