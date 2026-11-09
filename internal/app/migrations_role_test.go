package app_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/victorradael/condoguard/api/internal/app"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/middleware"
)

// splitRoles mirrors docker/postgres/initdb/10-papeis.sh of the monorepo: an
// owner role that holds the schema and an app role with only DML. It returns
// the owner and app connection strings for a fresh database.
func splitRoles(t *testing.T) (ownerURL, appURL string) {
	t.Helper()
	dbURL := dbtest.NewDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b := make([]byte, 4)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)
	owner, appRole := "cg_owner_"+suffix, "cg_app_"+suffix
	const pw = "test-password"

	admin, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	u, _ := url.Parse(dbURL)
	dbName := strings.TrimPrefix(u.Path, "/")
	for _, q := range []string{
		"CREATE ROLE " + owner + " LOGIN PASSWORD '" + pw + "' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS",
		"CREATE ROLE " + appRole + " LOGIN PASSWORD '" + pw + "' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS",
		"ALTER DATABASE " + dbName + " OWNER TO " + owner,
		"REVOKE ALL ON DATABASE " + dbName + " FROM PUBLIC",
		"GRANT CONNECT, TEMPORARY ON DATABASE " + dbName + " TO " + appRole,
		"ALTER SCHEMA public OWNER TO " + owner,
		"REVOKE ALL ON SCHEMA public FROM PUBLIC",
		"GRANT USAGE ON SCHEMA public TO " + appRole,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + owner + " IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO " + appRole,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + owner + " IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO " + appRole,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + owner + " IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO " + appRole,
	} {
		if _, err := admin.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// Runs after dbtest drops the database (cleanups are LIFO), so the roles own nothing.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(ctx, dbtest.URL(t))
		if err != nil {
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		_, _ = c.Exec(ctx, "DROP ROLE IF EXISTS "+appRole)
		_, _ = c.Exec(ctx, "DROP ROLE IF EXISTS "+owner)
	})

	as := func(role string) string {
		v := *u
		v.User = url.UserPassword(role, pw)
		return v.String()
	}
	return as(owner), as(appRole)
}

// The monorepo runs the API as a role without DDL and migrates as the schema
// owner: startup must use DATABASE_MIGRATIONS_URL for goose.
func TestNew_MigratesWithOwnerRole(t *testing.T) {
	ownerURL, appURL := splitRoles(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := app.Config{DatabaseURL: appURL, MigrateOnStart: true, JWTSecret: testSecret, CondominiumID: "c", LoginMaxAttempts: 5, LoginLockoutWindow: time.Minute}

	if a, err := app.New(context.Background(), base, logger, middleware.NewMetrics()); err == nil {
		a.Close()
		t.Fatal("the app role must not be able to run the migrations")
	}

	cfg := base
	cfg.MigrationsURL = ownerURL
	a, err := app.New(context.Background(), cfg, logger, middleware.NewMetrics())
	if err != nil {
		t.Fatalf("startup with DATABASE_MIGRATIONS_URL: %v", err)
	}
	defer a.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(context.Background()) }()
	var n int
	if err := c.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("the app role must read the tables created by the owner: %v", err)
	}
	if _, err := c.Exec(ctx, "CREATE TABLE x (id int)"); err == nil {
		t.Error("the app role must not have DDL")
	}
}
