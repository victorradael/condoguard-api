package user

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Repository defines the persistence contract for the user domain.
type Repository interface {
	Save(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindAll(ctx context.Context, p page.Params) ([]*User, int64, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
	// RevokeSessions revokes every refresh token of the user.
	RevokeSessions(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type memUser struct {
	User
	tenant string
	seq    int
}

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*memUser
	seq   int
}

// NewInMemoryRepository returns a thread-safe, tenant-scoped in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*memUser)}
}

func (r *inMemoryRepository) Save(ctx context.Context, u *User) error {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// E-mail is unique across the whole base, like the PostgreSQL index.
	for _, existing := range r.store {
		if existing.Email == u.Email {
			return ErrDuplicate
		}
	}

	r.seq++
	u.ID = uuid.NewString()
	r.store[u.ID] = &memUser{User: *u, tenant: t, seq: r.seq}
	return nil
}

func (r *inMemoryRepository) get(ctx context.Context, id string) (*memUser, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	u, ok := r.store[id]
	if !ok || u.tenant != t {
		return nil, ErrNotFound
	}
	return u, nil
}

func (r *inMemoryRepository) FindByID(ctx context.Context, id string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	cp := u.User
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(ctx context.Context, p page.Params) ([]*User, int64, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, 0, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []*memUser
	for _, u := range r.store {
		if u.tenant == t {
			all = append(all, u)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].seq < all[j].seq })
	window := page.Slice(all, p)
	out := make([]*User, len(window))
	for i, u := range window {
		cp := u.User
		out[i] = &cp
	}
	return out, int64(len(all)), nil
}

func (r *inMemoryRepository) Update(ctx context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, err := r.get(ctx, u.ID)
	if err != nil {
		return err
	}
	existing.User = *u
	return nil
}

func (r *inMemoryRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.get(ctx, id); err != nil {
		return err
	}
	delete(r.store, id)
	return nil
}

// RevokeSessions is a no-op: in memory, refresh tokens live in the auth
// package's own store, which this repository cannot reach.
func (r *inMemoryRepository) RevokeSessions(ctx context.Context, id string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, err := r.get(ctx, id)
	return err
}
