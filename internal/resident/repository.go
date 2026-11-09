package resident

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Repository defines the persistence contract for the resident domain.
// Every method is scoped to the condominium (tenant) in the context.
type Repository interface {
	Save(ctx context.Context, r *Resident) error
	FindByID(ctx context.Context, id string) (*Resident, error)
	// FindAll returns one page of units; ownerID "" means all of them.
	FindAll(ctx context.Context, ownerID string, p page.Params) ([]*Resident, int64, error)
	// IDsByOwner returns the IDs of the units owned by ownerID.
	IDsByOwner(ctx context.Context, ownerID string) ([]string, error)
	Update(ctx context.Context, r *Resident) error
	Delete(ctx context.Context, id string) error
	ExistsUnit(ctx context.Context, condominiumID, unitNumber, excludeID string) (bool, error)
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type memResident struct {
	Resident
	tenant string
	seq    int
}

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*memResident
	seq   int
}

// NewInMemoryRepository returns a thread-safe, tenant-scoped in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*memResident)}
}

func (r *inMemoryRepository) Save(ctx context.Context, res *Resident) error {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.tenant == t && existing.CondominiumID == res.CondominiumID && existing.UnitNumber == res.UnitNumber {
			return ErrDuplicate
		}
	}

	r.seq++
	res.ID = uuid.NewString()
	r.store[res.ID] = &memResident{Resident: *res, tenant: t, seq: r.seq}
	return nil
}

func (r *inMemoryRepository) get(ctx context.Context, id string) (*memResident, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	res, ok := r.store[id]
	if !ok || res.tenant != t {
		return nil, ErrNotFound
	}
	return res, nil
}

func (r *inMemoryRepository) FindByID(ctx context.Context, id string) (*Resident, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	cp := res.Resident
	return &cp, nil
}

func (r *inMemoryRepository) list(ctx context.Context, ownerID string) ([]*memResident, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	var list []*memResident
	for _, res := range r.store {
		if res.tenant == t && (ownerID == "" || res.OwnerID == ownerID) {
			list = append(list, res)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].seq < list[j].seq })
	return list, nil
}

func (r *inMemoryRepository) FindAll(ctx context.Context, ownerID string, p page.Params) ([]*Resident, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all, err := r.list(ctx, ownerID)
	if err != nil {
		return nil, 0, err
	}
	window := page.Slice(all, p)
	out := make([]*Resident, len(window))
	for i, res := range window {
		cp := res.Resident
		out[i] = &cp
	}
	return out, int64(len(all)), nil
}

func (r *inMemoryRepository) IDsByOwner(ctx context.Context, ownerID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all, err := r.list(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(all))
	for i, res := range all {
		ids[i] = res.ID
	}
	return ids, nil
}

func (r *inMemoryRepository) Update(ctx context.Context, res *Resident) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, err := r.get(ctx, res.ID)
	if err != nil {
		return err
	}
	existing.Resident = *res
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

func (r *inMemoryRepository) ExistsUnit(ctx context.Context, condominiumID, unitNumber, excludeID string) (bool, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, res := range r.store {
		if res.ID == excludeID || res.tenant != t {
			continue
		}
		if res.CondominiumID == condominiumID && res.UnitNumber == unitNumber {
			return true, nil
		}
	}
	return false, nil
}
