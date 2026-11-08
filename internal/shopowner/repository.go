package shopowner

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the shopowner domain.
type Repository interface {
	Save(ctx context.Context, s *ShopOwner) error
	FindByID(ctx context.Context, id string) (*ShopOwner, error)
	FindAll(ctx context.Context) ([]*ShopOwner, error)
	Update(ctx context.Context, s *ShopOwner) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*ShopOwner
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*ShopOwner)}
}

func (r *inMemoryRepository) Save(_ context.Context, s *ShopOwner) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.CNPJ == s.CNPJ {
			return ErrDuplicate
		}
	}

	s.ID = uuid.NewString()
	cp := *s
	r.store[s.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*ShopOwner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*ShopOwner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ShopOwner, 0, len(r.store))
	for _, s := range r.store {
		cp := *s
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, s *ShopOwner) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[s.ID]; !ok {
		return ErrNotFound
	}
	cp := *s
	r.store[s.ID] = &cp
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
