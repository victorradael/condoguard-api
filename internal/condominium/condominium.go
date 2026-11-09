// Package condominium holds the tenant itself: the condominium and the
// administradora (management company) it may belong to.
package condominium

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

// ErrNotFound is returned when the condominium of the context does not exist.
var ErrNotFound = errors.New("condominium: not found")

// Condominium is the tenant.
type Condominium struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	AdministratorID   string    `json:"administratorId,omitempty"`
	AdministratorName string    `json:"administratorName,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

// Repository persists condominiums.
type Repository interface {
	// Create makes a condominium (and its administradora, when administrator
	// is not empty) and returns it.
	Create(ctx context.Context, name, administrator string) (*Condominium, error)
	// Current returns the condominium of ctx.
	Current(ctx context.Context) (*Condominium, error)
	// Lookup resolves a deployment setting: the ID or the exact name.
	// "" and false when there is none.
	Lookup(ctx context.Context, ref string) (string, bool, error)
}

// ── PostgreSQL ────────────────────────────────────────────────────────────────

type postgresRepository struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

// NewPostgresRepository returns a Repository backed by the shared pgx pool.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool, q: db.Queries(pool)}
}

func (r *postgresRepository) Create(ctx context.Context, name, administrator string) (*Condominium, error) {
	id := uuid.New()
	// The new tenant is the scope of its own creation (see InsertCondominium).
	ctx = tenant.WithCondominiumID(ctx, id.String())
	err := db.InTx(ctx, r.pool, func(ctx context.Context) error {
		var adminID *uuid.UUID
		if administrator != "" {
			a := uuid.New()
			if err := r.q.InsertAdministrator(ctx, dbgen.InsertAdministratorParams{ID: a, Name: administrator}); err != nil {
				return err
			}
			adminID = &a
		}
		return r.q.InsertCondominium(ctx, dbgen.InsertCondominiumParams{ID: id, AdministratorID: adminID, Name: name})
	})
	if err != nil {
		return nil, err
	}
	return r.Current(ctx)
}

func (r *postgresRepository) Current(ctx context.Context) (*Condominium, error) {
	id, err := tenant.UUID(ctx)
	if err != nil {
		return nil, err
	}
	row, err := r.q.GetCondominium(ctx, id)
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c := &Condominium{ID: row.ID.String(), Name: row.Name, CreatedAt: row.CreatedAt}
	if row.AdministratorID != nil {
		c.AdministratorID = row.AdministratorID.String()
	}
	if row.AdministratorName != nil {
		c.AdministratorName = *row.AdministratorName
	}
	return c, nil
}

func (r *postgresRepository) Lookup(ctx context.Context, ref string) (string, bool, error) {
	var id *uuid.UUID
	if err := r.pool.QueryRow(ctx, `SELECT condominium_lookup($1)`, ref).Scan(&id); err != nil {
		return "", false, err
	}
	if id == nil {
		return "", false, nil
	}
	return id.String(), true, nil
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu   sync.Mutex
	list []*Condominium
}

// NewInMemoryRepository returns a thread-safe in-memory Repository.
func NewInMemoryRepository() Repository { return &inMemoryRepository{} }

func (r *inMemoryRepository) Create(_ context.Context, name, administrator string) (*Condominium, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Condominium{ID: uuid.NewString(), Name: name, AdministratorName: administrator, CreatedAt: time.Now()}
	if administrator != "" {
		c.AdministratorID = uuid.NewString()
	}
	r.list = append(r.list, c)
	cp := *c
	return &cp, nil
}

func (r *inMemoryRepository) Current(ctx context.Context) (*Condominium, error) {
	id, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.list {
		if c.ID == id {
			cp := *c
			return &cp, nil
		}
	}
	// Dev mode: the tenant of the in-memory repositories needs no record.
	return &Condominium{ID: id, Name: id}, nil
}

func (r *inMemoryRepository) Lookup(_ context.Context, ref string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.list {
		if c.ID == ref || c.Name == ref {
			return c.ID, true, nil
		}
	}
	return "", false, nil
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

// NewHandler serves GET /condominium: the condominium the token was issued for.
func NewHandler(repo Repository, authMW func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /condominium", func(w http.ResponseWriter, r *http.Request) {
		if _, err := authz.FromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		c, err := repo.Current(r.Context())
		switch {
		case errors.Is(err, ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "condominium not found"})
		case err != nil:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		default:
			writeJSON(w, http.StatusOK, c)
		}
	})
	return authMW(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
