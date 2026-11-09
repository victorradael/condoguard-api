// Package audit keeps the trail of who changed what: every change to the
// records of a condominium writes one entry with the actor, the action, the
// request origin and the value before and after. The table is insert-only
// (policies plus a trigger), and the entry is written in the same transaction
// as the change, so there is no change without a trail and no trail without a
// change.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Actions recorded for the cadastros.
const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// Entry is what a service reports about one change. Before is nil for a
// create, After is nil for a delete. Both must be free of secrets (password
// hashes, tokens): pass a sanitised view.
type Entry struct {
	Action     string
	Resource   string // "resident", "user", ...
	ResourceID string
	Before     any
	After      any
}

// Record is a stored entry.
type Record struct {
	ID          int64           `json:"id"`
	OccurredAt  time.Time       `json:"occurredAt"`
	ActorUserID string          `json:"actorUserId"`
	ActorRoles  []string        `json:"actorRoles"`
	Action      string          `json:"action"`
	Resource    string          `json:"resource"`
	ResourceID  string          `json:"resourceId"`
	IP          string          `json:"ip"`
	UserAgent   string          `json:"userAgent"`
	RequestID   string          `json:"requestId"`
	Before      json.RawMessage `json:"before,omitempty"`
	After       json.RawMessage `json:"after,omitempty"`
}

// Recorder writes audit entries. Callers run it inside db.Transactor.InTx,
// together with the change it describes.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// Reader lists the entries of the current condominium, newest first.
// resource and resourceID filter when not empty.
type Reader interface {
	List(ctx context.Context, resource, resourceID string, p page.Params) ([]Record, int64, error)
}

// Store is both.
type Store interface {
	Recorder
	Reader
}

func actor(ctx context.Context) (id *uuid.UUID, roles []string) {
	roles = middleware.RolesFromContext(ctx)
	if roles == nil {
		roles = []string{}
	}
	if u, err := uuid.Parse(middleware.UserIDFromContext(ctx)); err == nil {
		return &u, roles
	}
	return nil, roles // the operator tool (cmd/createadmin) is not a user
}

func marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// ── PostgreSQL ────────────────────────────────────────────────────────────────

type postgresStore struct {
	q *dbgen.Queries
}

// NewPostgresStore returns a Store backed by the shared pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{q: db.Queries(pool)}
}

func (s *postgresStore) Record(ctx context.Context, e Entry) error {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return err
	}
	before, err := marshal(e.Before)
	if err != nil {
		return fmt.Errorf("audit: before: %w", err)
	}
	after, err := marshal(e.After)
	if err != nil {
		return fmt.Errorf("audit: after: %w", err)
	}
	who, roles := actor(ctx)
	meta := middleware.RequestMetaFromContext(ctx)
	return s.q.InsertAudit(ctx, dbgen.InsertAuditParams{
		CondominiumID: condoID,
		ActorUserID:   who,
		ActorRoles:    roles,
		Action:        e.Action,
		Resource:      e.Resource,
		ResourceID:    e.ResourceID,
		Ip:            meta.IP,
		UserAgent:     meta.UserAgent,
		RequestID:     middleware.RequestIDFromContext(ctx),
		Before:        before,
		After:         after,
	})
}

func (s *postgresStore) List(ctx context.Context, resource, resourceID string, p page.Params) ([]Record, int64, error) {
	condoID, err := tenant.UUID(ctx)
	if err != nil {
		return nil, 0, err
	}
	var res, rid *string
	if resource != "" {
		res = &resource
	}
	if resourceID != "" {
		rid = &resourceID
	}
	rows, err := s.q.ListAudit(ctx, dbgen.ListAuditParams{CondominiumID: condoID, Resource: res, ResourceID: rid, PageLimit: p.Limit(), PageOffset: p.Offset()})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAudit(ctx, dbgen.CountAuditParams{CondominiumID: condoID, Resource: res, ResourceID: rid})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		rec := Record{ID: r.ID, OccurredAt: r.OccurredAt, ActorRoles: r.ActorRoles, Action: r.Action, Resource: r.Resource,
			ResourceID: r.ResourceID, IP: r.Ip, UserAgent: r.UserAgent, RequestID: r.RequestID, Before: r.Before, After: r.After}
		if r.ActorUserID != nil {
			rec.ActorUserID = r.ActorUserID.String()
		}
		out = append(out, rec)
	}
	return out, total, nil
}

// ── In-memory (dev / tests without a database) ────────────────────────────────

type memoryStore struct {
	mu   sync.Mutex
	next int64
	rows map[string][]Record // by condominium
}

// NewInMemoryStore returns a thread-safe in-memory Store.
func NewInMemoryStore() Store { return &memoryStore{rows: map[string][]Record{}} }

func (s *memoryStore) Record(ctx context.Context, e Entry) error {
	condo, err := tenant.CondominiumID(ctx)
	if err != nil {
		return err
	}
	before, err := marshal(e.Before)
	if err != nil {
		return err
	}
	after, err := marshal(e.After)
	if err != nil {
		return err
	}
	who, roles := actor(ctx)
	meta := middleware.RequestMetaFromContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	rec := Record{ID: s.next, OccurredAt: time.Now(), ActorRoles: roles, Action: e.Action, Resource: e.Resource,
		ResourceID: e.ResourceID, IP: meta.IP, UserAgent: meta.UserAgent, RequestID: middleware.RequestIDFromContext(ctx),
		Before: before, After: after}
	if who != nil {
		rec.ActorUserID = who.String()
	}
	s.rows[condo] = append(s.rows[condo], rec)
	return nil
}

func (s *memoryStore) List(ctx context.Context, resource, resourceID string, p page.Params) ([]Record, int64, error) {
	condo, err := tenant.CondominiumID(ctx)
	if err != nil {
		return nil, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []Record
	for i := len(s.rows[condo]) - 1; i >= 0; i-- {
		r := s.rows[condo][i]
		if (resource == "" || r.Resource == resource) && (resourceID == "" || r.ResourceID == resourceID) {
			all = append(all, r)
		}
	}
	total := int64(len(all))
	lo := min(int(p.Offset()), len(all))
	hi := min(lo+int(p.Limit()), len(all))
	return all[lo:hi], total, nil
}

// Discard drops every entry (tests of services that do not care about the trail).
type Discard struct{}

// Record does nothing.
func (Discard) Record(context.Context, Entry) error { return nil }

// Trail ties a change to its audit entry: both are written in one
// transaction, so either both happen or neither does.
type Trail struct {
	Rec Recorder
	Tx  db.Transactor
}

// NewTrail returns a Trail that records nothing and opens no transaction.
// Services start with it; the router installs the real one with WithAudit.
func NewTrail() Trail { return Trail{Rec: Discard{}, Tx: db.NoTx{}} }

// Atomic runs change in a transaction and records the entry it returns.
func (t Trail) Atomic(ctx context.Context, change func(ctx context.Context) (Entry, error)) error {
	return t.Tx.InTx(ctx, func(ctx context.Context) error {
		e, err := change(ctx)
		if err != nil {
			return err
		}
		return t.Rec.Record(ctx, e)
	})
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

// NewHandler serves GET /audit-log?resource=&resourceId=&page=&pageSize=:
// the trail of the caller's condominium, newest first. Needs audit.read.
func NewHandler(r Reader, authMW func(http.Handler) http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /audit-log", func(w http.ResponseWriter, req *http.Request) {
		if _, err := authz.Require(req.Context(), authz.PermAuditRead); err != nil {
			status, msg, _ := authz.Status(err)
			writeJSON(w, status, map[string]string{"error": msg})
			return
		}
		p, err := page.FromRequest(req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		q := req.URL.Query()
		list, total, err := r.List(req.Context(), q.Get("resource"), q.Get("resourceId"), p)
		if err != nil {
			slog.ErrorContext(req.Context(), "audit: list", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, http.StatusOK, page.New(list, p, total))
	})
	return authMW(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
