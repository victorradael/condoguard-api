package expense

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the expense domain.
type Repository interface {
	Save(ctx context.Context, e *Expense) error
	FindByID(ctx context.Context, id string) (*Expense, error)
	FindAll(ctx context.Context, filter Filter) ([]*Expense, error)
	Update(ctx context.Context, e *Expense) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*Expense
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*Expense)}
}

func (r *inMemoryRepository) Save(_ context.Context, e *Expense) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.ID = uuid.NewString()
	cp := *e
	r.store[e.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context, f Filter) ([]*Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Expense, 0, len(r.store))
	for _, e := range r.store {
		if f.From != nil && e.DueDate.Before(*f.From) {
			continue
		}
		if f.To != nil && e.DueDate.After(*f.To) {
			continue
		}
		cp := *e
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, e *Expense) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[e.ID]; !ok {
		return ErrNotFound
	}
	cp := *e
	r.store[e.ID] = &cp
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
