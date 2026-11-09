package expense

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Repository defines the persistence contract for the expense domain.
// Every method is scoped to the condominium (tenant) in the context.
type Repository interface {
	Save(ctx context.Context, e *Expense) error
	FindByID(ctx context.Context, id string) (*Expense, error)
	// FindAll returns one page of the expenses matching filter, in due-date order.
	FindAll(ctx context.Context, filter Filter, p page.Params) ([]*Expense, int64, error)
	Update(ctx context.Context, e *Expense) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type memExpense struct {
	Expense
	tenant string
}

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*memExpense
}

// NewInMemoryRepository returns a thread-safe, tenant-scoped in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*memExpense)}
}

func (r *inMemoryRepository) Save(ctx context.Context, e *Expense) error {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e.ID = uuid.NewString()
	r.store[e.ID] = &memExpense{Expense: *e, tenant: t}
	return nil
}

func (r *inMemoryRepository) get(ctx context.Context, id string) (*memExpense, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	e, ok := r.store[id]
	if !ok || e.tenant != t {
		return nil, ErrNotFound
	}
	return e, nil
}

func (r *inMemoryRepository) FindByID(ctx context.Context, id string) (*Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	cp := e.Expense
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(ctx context.Context, f Filter, p page.Params) ([]*Expense, int64, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, 0, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Expense, 0, len(r.store))
	for _, e := range r.store {
		if e.tenant != t || !f.Scope.Sees(e.ResidentID, e.ShopOwnerID) {
			continue
		}
		if f.From != nil && e.DueDate.Before(*f.From) {
			continue
		}
		if f.To != nil && e.DueDate.After(*f.To) {
			continue
		}
		cp := e.Expense
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].DueDate.Equal(list[j].DueDate) {
			return list[i].DueDate.Before(list[j].DueDate)
		}
		return list[i].ID < list[j].ID
	})
	return page.Slice(list, p), int64(len(list)), nil
}

func (r *inMemoryRepository) Update(ctx context.Context, e *Expense) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, err := r.get(ctx, e.ID)
	if err != nil {
		return err
	}
	existing.Expense = *e
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
