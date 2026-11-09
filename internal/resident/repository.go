package resident

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Repository defines the persistence contract for the resident domain.
type Repository interface {
	Save(ctx context.Context, r *Resident) error
	FindByID(ctx context.Context, id string) (*Resident, error)
	FindAll(ctx context.Context) ([]*Resident, error)
	Update(ctx context.Context, r *Resident) error
	Delete(ctx context.Context, id string) error
	ExistsUnit(ctx context.Context, condominiumID, unitNumber, excludeID string) (bool, error)
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*Resident
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*Resident)}
}

func (r *inMemoryRepository) Save(_ context.Context, res *Resident) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.CondominiumID == res.CondominiumID && existing.UnitNumber == res.UnitNumber {
			return ErrDuplicate
		}
	}

	res.ID = uuid.NewString()
	cp := *res
	r.store[res.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*Resident, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *res
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*Resident, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Resident, 0, len(r.store))
	for _, res := range r.store {
		cp := *res
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, res *Resident) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[res.ID]; !ok {
		return ErrNotFound
	}
	cp := *res
	r.store[res.ID] = &cp
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

func (r *inMemoryRepository) ExistsUnit(_ context.Context, condominiumID, unitNumber, excludeID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, res := range r.store {
		if res.ID == excludeID {
			continue
		}
		if res.CondominiumID == condominiumID && res.UnitNumber == unitNumber {
			return true, nil
		}
	}
	return false, nil
}
