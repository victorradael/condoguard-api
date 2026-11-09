package user

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the user domain.
type Repository interface {
	Save(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindAll(ctx context.Context) ([]*User, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*User
	seq   int
}

// NewInMemoryRepository returns a thread-safe in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*User)}
}

func (r *inMemoryRepository) Save(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.Email == u.Email {
			return ErrDuplicate
		}
	}

	r.seq++
	u.ID = uuid.NewString()
	cp := *u
	r.store[u.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*User, 0, len(r.store))
	for _, u := range r.store {
		cp := *u
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[u.ID]; !ok {
		return ErrNotFound
	}
	cp := *u
	r.store[u.ID] = &cp
	return nil
}

func (r *inMemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[id]; !ok {
		return ErrNotFound
	}
	delete(r.store, id)
	return nil
}
