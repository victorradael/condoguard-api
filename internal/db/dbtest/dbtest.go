// Package dbtest gives repository tests a real, migrated PostgreSQL database.
//
// Tests are skipped unless TEST_DATABASE_URL points to a PostgreSQL server
// whose user may CREATE DATABASE (the CI postgres service, or `make test-db-up`).
// Each call creates a throwaway database, so test packages running in
// parallel never see each other's rows.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db"
)

// EnvURL is the environment variable holding the admin connection string.
const EnvURL = "TEST_DATABASE_URL"

// URL returns TEST_DATABASE_URL or skips the test.
func URL(t testing.TB) string {
	t.Helper()
	u := os.Getenv(EnvURL)
	if u == "" {
		t.Skip(EnvURL + " not set — skipping PostgreSQL test")
	}
	return u
}

// NewDatabase creates an empty database and returns its connection string.
// The database is dropped when the test finishes.
func NewDatabase(t testing.TB) string {
	t.Helper()
	adminURL := URL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("dbtest: connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "condoguard_test_" + hex.EncodeToString(b)
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("dbtest: create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, cerr := pgx.Connect(ctx, adminURL)
		if cerr != nil {
			t.Logf("dbtest: cleanup connect: %v", cerr)
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		if _, cerr = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); cerr != nil {
			t.Logf("dbtest: drop database: %v", cerr)
		}
	})

	u, err := url.Parse(adminURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatalf("dbtest: %s must be a postgres:// URL", EnvURL)
	}
	u.Path = "/" + name
	return u.String()
}

// Env is a fresh, fully migrated database seen the way production sees it:
// Pool connects as a role that owns nothing, is no superuser and has no
// BYPASSRLS, so Row-Level Security applies to it. Admin connects as the
// administrative user of TEST_DATABASE_URL and ignores RLS (fixtures,
// assertions about what is really stored).
type Env struct {
	Pool  *pgxpool.Pool
	Admin *pgxpool.Pool
	// AppURL is the connection string of Pool.
	AppURL string
	t      testing.TB
	condos map[string]string
}

// NewEnv creates the database, migrates it and creates the restricted role.
func NewEnv(t testing.TB) *Env {
	t.Helper()
	ctx := context.Background()
	adminURL := URL(t)

	b := make([]byte, 5)
	_, _ = rand.Read(b)
	role := "cg_app_" + hex.EncodeToString(b)
	const pw = "test-password"

	// Registered before NewDatabase so that, cleanups being LIFO, the role is
	// dropped after the database (and so owns and holds nothing).
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		_, _ = c.Exec(ctx, "DROP ROLE IF EXISTS "+role)
	})
	dbURL := NewDatabase(t)

	admin, err := db.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(admin.Close)
	if err := db.MigrateUp(ctx, admin); err != nil {
		t.Fatalf("dbtest: %v", err)
	}

	u, _ := url.Parse(dbURL)
	dbName := strings.TrimPrefix(u.Path, "/")
	for _, q := range []string{
		"CREATE ROLE " + role + " LOGIN PASSWORD '" + pw + "' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS",
		"GRANT CONNECT ON DATABASE " + dbName + " TO " + role,
		"GRANT USAGE ON SCHEMA public TO " + role,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + role,
		"REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM " + role,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + role,
		"GRANT EXECUTE ON FUNCTION auth_user_by_email(text), auth_memberships(uuid), auth_refresh_condominium(bytea), condominium_lookup(text), remove_membership(uuid) TO " + role,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("dbtest: %s: %v", q, err)
		}
	}

	au := *u
	au.User = url.UserPassword(role, pw)
	pool, err := db.NewPool(ctx, au.String())
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(pool.Close)
	return &Env{Pool: pool, Admin: admin, AppURL: au.String(), t: t, condos: map[string]string{}}
}

// Condo returns the ID of the condominium called name, creating it the first time.
func (e *Env) Condo(name string) string {
	e.t.Helper()
	if id, ok := e.condos[name]; ok {
		return id
	}
	var id string
	err := e.Admin.QueryRow(context.Background(), `INSERT INTO condominiums (name) VALUES ($1) RETURNING id::text`, name).Scan(&id)
	if err != nil {
		e.t.Fatalf("dbtest: create condominium %q: %v", name, err)
	}
	e.condos[name] = id
	return id
}

// Fixed condominium IDs for tests that need the same tenants in-memory and in
// PostgreSQL (tokens carry them).
const (
	CondoA = "a0000000-0000-4000-8000-00000000000a"
	CondoB = "b0000000-0000-4000-8000-00000000000b"
)

// SeedCondo creates a condominium with a known ID.
func (e *Env) SeedCondo(id string) {
	e.t.Helper()
	if _, err := e.Admin.Exec(context.Background(), `INSERT INTO condominiums (id, name) VALUES ($1::uuid, $1::text) ON CONFLICT DO NOTHING`, id); err != nil {
		e.t.Fatalf("dbtest: seed condominium %s: %v", id, err)
	}
}

// NewPool returns a pool, as the restricted role, connected to a fresh, fully
// migrated database holding the condominiums with the given IDs. Use NewEnv
// when the test needs more control.
func NewPool(t testing.TB, condos ...string) *pgxpool.Pool {
	t.Helper()
	e := NewEnv(t)
	for _, id := range condos {
		e.SeedCondo(id)
	}
	return e.Pool
}
