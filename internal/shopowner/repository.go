package shopowner

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Repository defines the persistence contract for the shopowner domain.
// Every method is scoped to the condominium (tenant) in the context.
type Repository interface {
	Save(ctx context.Context, s *ShopOwner) error
	FindByID(ctx context.Context, id string) (*ShopOwner, error)
	// FindAll returns one page of shops; ownerID "" means all of them.
	FindAll(ctx context.Context, ownerID string, p page.Params) ([]*ShopOwner, int64, error)
	// IDsByOwner returns the IDs of the shops owned by ownerID.
	IDsByOwner(ctx context.Context, ownerID string) ([]string, error)
	Update(ctx context.Context, s *ShopOwner) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type memShop struct {
	ShopOwner
	tenant string
	seq    int
}

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*memShop
	seq   int
}

// NewInMemoryRepository returns a thread-safe, tenant-scoped in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*memShop)}
}

func (r *inMemoryRepository) Save(ctx context.Context, res *ShopOwner) error {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.tenant == t && existing.CNPJ == res.CNPJ {
			return ErrDuplicate
		}
	}

	r.seq++
	res.ID = uuid.NewString()
	r.store[res.ID] = &memShop{ShopOwner: *res, tenant: t, seq: r.seq}
	return nil
}

func (r *inMemoryRepository) get(ctx context.Context, id string) (*memShop, error) {
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

func (r *inMemoryRepository) FindByID(ctx context.Context, id string) (*ShopOwner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	cp := res.ShopOwner
	return &cp, nil
}

func (r *inMemoryRepository) list(ctx context.Context, ownerID string) ([]*memShop, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	var list []*memShop
	for _, res := range r.store {
		if res.tenant == t && (ownerID == "" || res.OwnerID == ownerID) {
			list = append(list, res)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].seq < list[j].seq })
	return list, nil
}

func (r *inMemoryRepository) FindAll(ctx context.Context, ownerID string, p page.Params) ([]*ShopOwner, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all, err := r.list(ctx, ownerID)
	if err != nil {
		return nil, 0, err
	}
	window := page.Slice(all, p)
	out := make([]*ShopOwner, len(window))
	for i, res := range window {
		cp := res.ShopOwner
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

func (r *inMemoryRepository) Update(ctx context.Context, res *ShopOwner) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, err := r.get(ctx, res.ID)
	if err != nil {
		return err
	}
	existing.ShopOwner = *res
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
