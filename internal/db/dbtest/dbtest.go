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

// NewPool returns a pool connected to a fresh, fully migrated database.
func NewPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := db.NewPool(ctx, NewDatabase(t))
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	return pool
}
