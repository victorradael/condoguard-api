package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

// Scoped is the second layer of tenant isolation. It implements dbgen.DBTX on
// top of the pool: every statement runs in a transaction that first tells
// PostgreSQL which condominium (app.condominium_id) and user (app.user_id)
// the request belongs to, and the Row-Level Security policies do the rest.
// Without a tenant in the context the setting stays empty and the policies
// show no row (fail closed). The first layer is the repositories, which also
// put the condominium in every WHERE clause.
//
// Inside InTx every statement joins the surrounding transaction instead.
type Scoped struct {
	pool *pgxpool.Pool
}

// NewScoped wraps the pool.
func NewScoped(pool *pgxpool.Pool) *Scoped { return &Scoped{pool: pool} }

// Queries returns the sqlc queries running through the tenant scope.
func Queries(pool *pgxpool.Pool) *dbgen.Queries { return dbgen.New(NewScoped(pool)) }

type txKey struct{}

type ctxTx struct {
	tx     pgx.Tx
	condo  string
	userID string
}

// scope returns the values handed to PostgreSQL for ctx.
func scope(ctx context.Context) (condo, userID string) {
	condo, _ = tenant.CondominiumID(ctx)
	return condo, tenant.UserID(ctx)
}

func begin(ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	condo, userID := scope(ctx)
	if _, err = tx.Exec(ctx, `SELECT set_config('app.condominium_id', $1, true), set_config('app.user_id', $2, true)`, condo, userID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("db: set tenant: %w", err)
	}
	return tx, nil
}

// InTx runs fn in one transaction scoped to the tenant and user in ctx; every
// repository call made with the ctx given to fn joins it. A non-nil error
// from fn rolls everything back. Nested calls reuse the outer transaction.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*ctxTx); ok {
		return fn(ctx)
	}
	tx, err := begin(ctx, pool)
	if err != nil {
		return err
	}
	condo, userID := scope(ctx)
	inner := context.WithValue(ctx, txKey{}, &ctxTx{tx: tx, condo: condo, userID: userID})
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err = fn(inner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Transactor groups several repository calls into one atomic unit.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// PoolTx is the Transactor of a pgx pool.
type PoolTx struct{ Pool *pgxpool.Pool }

// InTx runs fn in a tenant-scoped transaction.
func (p PoolTx) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return InTx(ctx, p.Pool, fn)
}

// NoTx is the Transactor of the in-memory repositories: it just runs fn.
type NoTx struct{}

// InTx runs fn.
func (NoTx) InTx(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

// ErrTenantChanged is returned when a statement inside InTx runs with another
// tenant than the transaction was opened for.
var ErrTenantChanged = errors.New("db: tenant changed inside a transaction")

func (s *Scoped) current(ctx context.Context) (*ctxTx, error) {
	t, ok := ctx.Value(txKey{}).(*ctxTx)
	if !ok {
		return nil, nil
	}
	if condo, userID := scope(ctx); condo != t.condo || userID != t.userID {
		return nil, ErrTenantChanged
	}
	return t, nil
}

// Exec implements dbgen.DBTX.
func (s *Scoped) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t, err := s.current(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	if t != nil {
		return t.tx.Exec(ctx, sql, args...)
	}
	tx, err := begin(ctx, s.pool)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return tag, err
	}
	return tag, tx.Commit(ctx)
}

// Query implements dbgen.DBTX. The transaction ends when the rows are closed.
func (s *Scoped) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	t, err := s.current(ctx)
	if err != nil {
		return nil, err
	}
	if t != nil {
		return t.tx.Query(ctx, sql, args...)
	}
	tx, err := begin(ctx, s.pool)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return nil, err
	}
	return &scopedRows{Rows: rows, tx: tx, ctx: ctx}, nil
}

// QueryRow implements dbgen.DBTX.
func (s *Scoped) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	rows, err := s.Query(ctx, sql, args...)
	return &scopedRow{rows: rows, err: err}
}

type scopedRows struct {
	pgx.Rows
	tx        pgx.Tx
	ctx       context.Context
	closed    bool
	commitErr error
}

func (r *scopedRows) Close() {
	if r.closed {
		return
	}
	r.closed = true
	r.Rows.Close()
	if r.Rows.Err() != nil {
		_ = r.tx.Rollback(context.WithoutCancel(r.ctx))
		return
	}
	r.commitErr = r.tx.Commit(r.ctx)
}

func (r *scopedRows) Err() error {
	if err := r.Rows.Err(); err != nil {
		return err
	}
	return r.commitErr
}

type scopedRow struct {
	rows pgx.Rows
	err  error
}

func (r *scopedRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	defer r.rows.Close()
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	if err := r.rows.Scan(dest...); err != nil {
		return err
	}
	r.rows.Close()
	return r.rows.Err()
}
