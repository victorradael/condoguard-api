package notification

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the notification domain.
type Repository interface {
	Save(ctx context.Context, n *Notification) error
	FindByID(ctx context.Context, id string) (*Notification, error)
	FindAll(ctx context.Context) ([]*Notification, error)
	Update(ctx context.Context, n *Notification) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*Notification
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*Notification)}
}

func (r *inMemoryRepository) Save(_ context.Context, n *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n.ID = uuid.NewString()
	cp := *n
	if cp.ResidentIDs == nil {
		cp.ResidentIDs = []string{}
	}
	if cp.ShopOwnerIDs == nil {
		cp.ShopOwnerIDs = []string{}
	}
	r.store[n.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *n
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*Notification, 0, len(r.store))
	for _, n := range r.store {
		cp := *n
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, n *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[n.ID]; !ok {
		return ErrNotFound
	}
	cp := *n
	r.store[n.ID] = &cp
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
