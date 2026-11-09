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
// owner role that owns the database and the public schema, and a runtime role
// with only USAGE on it (no DDL). It returns the URLs of both roles.
func splitRoles(t *testing.T) (ownerURL, appURL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminURL := dbtest.URL(t)

	b := make([]byte, 4)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)
	owner, appRole, pw := "cg_owner_"+suffix, "cg_app_"+suffix, "pw-"+suffix

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	for _, r := range []string{owner, appRole} {
		if _, err := admin.Exec(ctx, "CREATE ROLE "+r+" LOGIN PASSWORD '"+pw+"' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS"); err != nil {
			t.Fatal(err)
		}
	}
	// Registered before NewDatabase, so it runs after the database is dropped.
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), adminURL)
		if err != nil {
			t.Logf("drop roles: %v", err)
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		for _, r := range []string{appRole, owner} {
			if _, err := c.Exec(context.Background(), "DROP ROLE IF EXISTS "+r); err != nil {
				t.Logf("drop role %s: %v", r, err)
			}
		}
	})

	dbURL := dbtest.NewDatabase(t)
	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	c, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(context.Background()) }()
	for _, q := range []string{
		"ALTER DATABASE " + name + " OWNER TO " + owner,
		"REVOKE ALL ON DATABASE " + name + " FROM PUBLIC",
		"GRANT CONNECT, TEMPORARY ON DATABASE " + name + " TO " + appRole,
		"ALTER SCHEMA public OWNER TO " + owner,
		"REVOKE ALL ON SCHEMA public FROM PUBLIC",
		"GRANT USAGE ON SCHEMA public TO " + appRole,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + owner + " IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO " + appRole,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + owner + " IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO " + appRole,
	} {
		if _, err := c.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	withUser := func(user string) string {
		v := *u
		v.User = url.UserPassword(user, pw)
		return v.String()
	}
	return withUser(owner), withUser(appRole)
}

// FAB-15: in the compose the API runs as a role without DDL rights, so the
// migrations must run as the schema owner (DATABASE_MIGRATIONS_URL).
func TestNew_MigratesAsSchemaOwner(t *testing.T) {
	ownerURL, appURL := splitRoles(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := app.Config{DatabaseURL: appURL, MigrateOnStart: true, JWTSecret: testSecret, CondominiumID: "c", LoginMaxAttempts: 5, LoginLockoutWindow: time.Minute}

	if a, err := app.New(context.Background(), base, logger, middleware.NewMetrics()); err == nil {
		a.Close()
		t.Fatal("runtime role must not be able to migrate (it has no CREATE on public)")
	}

	cfg := base
	cfg.DatabaseMigrationsURL = ownerURL
	a, err := app.New(context.Background(), cfg, logger, middleware.NewMetrics())
	if err != nil {
		t.Fatalf("migrate as owner: %v", err)
	}
	defer a.Close()

	// The runtime role reads and writes the tables the owner created.
	c, err := pgx.Connect(context.Background(), appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(context.Background()) }()
	var n int
	if err := c.QueryRow(context.Background(), "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("runtime role reads users: %v", err)
	}
}
