package notification

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Repository defines the persistence contract for the notification domain.
// Every method is scoped to the condominium (tenant) in the context.
// readerID selects whose read state is returned in Read/ReadAt.
type Repository interface {
	Save(ctx context.Context, n *Notification) error
	FindByID(ctx context.Context, id, readerID string) (*Notification, error)
	// FindAll returns one page of the notifications visible in scope, newest first.
	FindAll(ctx context.Context, scope authz.Scope, readerID string, p page.Params) ([]*Notification, int64, error)
	// Update changes the message and the recipients.
	Update(ctx context.Context, n *Notification) error
	Delete(ctx context.Context, id string) error
	// MarkRead records that userID read the notification and returns when
	// they first did. It is idempotent.
	MarkRead(ctx context.Context, id, userID string) (time.Time, error)
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type memNotification struct {
	Notification
	tenant string
	reads  map[string]time.Time // userID → first read
}

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*memNotification
}

// NewInMemoryRepository returns a thread-safe, tenant-scoped in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*memNotification)}
}

func (r *inMemoryRepository) Save(ctx context.Context, n *Notification) error {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n.ID = uuid.NewString()
	cp := *n
	cp.Read, cp.ReadAt = false, nil
	if cp.ResidentIDs == nil {
		cp.ResidentIDs = []string{}
	}
	if cp.ShopOwnerIDs == nil {
		cp.ShopOwnerIDs = []string{}
	}
	r.store[n.ID] = &memNotification{Notification: cp, tenant: t, reads: map[string]time.Time{}}
	return nil
}

func (r *inMemoryRepository) get(ctx context.Context, id string) (*memNotification, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	n, ok := r.store[id]
	if !ok || n.tenant != t {
		return nil, ErrNotFound
	}
	return n, nil
}

func (m *memNotification) view(readerID string) *Notification {
	cp := m.Notification
	if at, ok := m.reads[readerID]; ok {
		cp.Read, cp.ReadAt = true, &at
	}
	return &cp
}

func (r *inMemoryRepository) FindByID(ctx context.Context, id, readerID string) (*Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, err := r.get(ctx, id)
	if err != nil {
		return nil, err
	}
	return n.view(readerID), nil
}

func (r *inMemoryRepository) FindAll(ctx context.Context, scope authz.Scope, readerID string, p page.Params) ([]*Notification, int64, error) {
	t, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, 0, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*Notification, 0, len(r.store))
	for _, n := range r.store {
		if n.tenant == t && scope.SeesAny(n.ResidentIDs, n.ShopOwnerIDs) {
			list = append(list, n.view(readerID))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.After(list[j].CreatedAt)
		}
		return list[i].ID < list[j].ID
	})
	return page.Slice(list, p), int64(len(list)), nil
}

func (r *inMemoryRepository) Update(ctx context.Context, n *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, err := r.get(ctx, n.ID)
	if err != nil {
		return err
	}
	existing.Message = n.Message
	existing.ResidentIDs = n.ResidentIDs
	existing.ShopOwnerIDs = n.ShopOwnerIDs
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

func (r *inMemoryRepository) MarkRead(ctx context.Context, id, userID string) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.get(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	if at, ok := n.reads[userID]; ok {
		return at, nil
	}
	at := time.Now().UTC()
	n.reads[userID] = at
	return at, nil
}
