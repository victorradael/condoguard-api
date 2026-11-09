package db_test

import (
	"context"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
)

var tables = []string{"users", "residents", "shop_owners", "expenses", "notifications", "refresh_tokens", "notification_reads",
	"administrators", "condominiums", "memberships", "audit_log"}

// Tables that are not rows of one condominium: the accounts (global identity,
// visible through the membership), the condominiums themselves and their
// administradoras.
var globalTables = map[string]bool{"users": true, "condominiums": true, "administrators": true}

// goose up → down to zero → up again must work on an empty database.
func TestMigrations_UpDownUp_FromScratch(t *testing.T) {
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dbtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	count := func() int {
		var n int
		err := pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ANY($1)`,
			tables).Scan(&n)
		if err != nil {
			t.Fatalf("count tables: %v", err)
		}
		return n
	}

	for round := 1; round <= 2; round++ {
		if err := db.MigrateUp(ctx, pool); err != nil {
			t.Fatalf("round %d up: %v", round, err)
		}
		if n := count(); n != len(tables) {
			t.Fatalf("round %d after up: expected %d tables, got %d", round, len(tables), n)
		}
		if v, err := db.MigrationVersion(ctx, pool); err != nil || v < 1 {
			t.Fatalf("round %d: expected version >= 1, got %d (%v)", round, v, err)
		}

		if err := db.MigrateDownTo(ctx, pool, 0); err != nil {
			t.Fatalf("round %d down: %v", round, err)
		}
		if n := count(); n != 0 {
			t.Fatalf("round %d after down: expected 0 tables, got %d", round, n)
		}
		if v, err := db.MigrationVersion(ctx, pool); err != nil || v != 0 {
			t.Fatalf("round %d: expected version 0, got %d (%v)", round, v, err)
		}
	}
}

// Every table is under Row-Level Security, and every tenant table carries a
// NOT NULL condominium_id with a foreign key to condominiums.
func TestSchema_EveryTableIsTenantScopedAndUnderRLS(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)

	rows, err := pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, col.is_nullable, col.udt_name,
		       EXISTS (SELECT 1 FROM pg_constraint k
		               WHERE k.conrelid = c.oid AND k.contype = 'f' AND k.confrelid = 'condominiums'::regclass)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		LEFT JOIN information_schema.columns col
		  ON col.table_schema = 'public' AND col.table_name = c.relname AND col.column_name = 'condominium_id'
		WHERE c.relkind = 'r' AND c.relname <> 'goose_db_version'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var (
			table    string
			rls      bool
			hasFK    bool
			nullable *string
			udtName  *string
		)
		if err := rows.Scan(&table, &rls, &nullable, &udtName, &hasFK); err != nil {
			t.Fatal(err)
		}
		seen++
		if !rls {
			t.Errorf("table %s: row-level security is not enabled", table)
		}
		if globalTables[table] {
			continue
		}
		switch {
		case nullable == nil:
			t.Errorf("table %s has no condominium_id column", table)
		case *nullable != "NO":
			t.Errorf("table %s: condominium_id must be NOT NULL", table)
		case *udtName != "uuid":
			t.Errorf("table %s: condominium_id must be uuid, is %s", table, *udtName)
		case !hasFK:
			t.Errorf("table %s: condominium_id has no foreign key to condominiums", table)
		}
	}
	if seen != len(tables) {
		t.Errorf("expected %d tables, found %d", len(tables), seen)
	}
}

func TestParseID(t *testing.T) {
	if _, ok := db.ParseID("2b1e4f1c-8a0f-4c1e-9d43-0d8f5a8b9e10"); !ok {
		t.Error("canonical UUID must parse")
	}
	for _, bad := range []string{"", "nonexistent", "000000000000000000000000", "{2b1e4f1c-8a0f-4c1e-9d43-0d8f5a8b9e10}", "2b1e4f1c8a0f4c1e9d430d8f5a8b9e10"} {
		if _, ok := db.ParseID(bad); ok {
			t.Errorf("ParseID(%q) should fail", bad)
		}
	}
}

// The 1a data (text condominium, ROLE_* roles) survives the move to the tenant model.
func TestMigration00003_KeepsTheDataOf1a(t *testing.T) {
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dbtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.MigrateUp(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateDownTo(ctx, pool, 2); err != nil {
		t.Fatalf("down to 1a: %v", err)
	}

	for _, q := range []string{
		`INSERT INTO users (id, condominium_id, username, email, password_hash, roles) VALUES
		   ('11111111-1111-4111-8111-111111111111', 'default', 'sindico', 'sindico@exemplo.test', 'h', '{ROLE_ADMIN}'),
		   ('22222222-2222-4222-8222-222222222222', 'default', 'ana', 'ana@exemplo.test', 'h', '{ROLE_USER}'),
		   ('33333333-3333-4333-8333-333333333333', 'outro', 'bia', 'bia@exemplo.test', 'h', '{}')`,
		`INSERT INTO residents (condominium_id, condominium_ref, unit_number, owner_id) VALUES ('default', 'b1', '101', 'x'), ('outro', 'b1', '101', 'y')`,
		`INSERT INTO expenses (condominium_id, description, amount_cents, due_date) VALUES ('default', 'Água', 1, now())`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := db.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("up to 1b with data: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM condominiums WHERE name IN ('default', 'outro')`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("condominiums created from the old tenant values: %d (%v)", n, err)
	}
	rows := map[string]string{}
	r, err := pool.Query(ctx, `SELECT u.username, m.roles::text || '@' || c.name FROM users u JOIN memberships m ON m.user_id = u.id JOIN condominiums c ON c.id = m.condominium_id`)
	if err != nil {
		t.Fatal(err)
	}
	for r.Next() {
		var k, v string
		if err := r.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		rows[k] = v
	}
	r.Close()
	want := map[string]string{"sindico": "{sindico}@default", "ana": "{morador}@default", "bia": "{morador}@outro"}
	for k, v := range want {
		if rows[k] != v {
			t.Errorf("membership of %s: got %q, want %q", k, rows[k], v)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM residents r JOIN condominiums c ON c.id = r.condominium_id WHERE c.name = 'default' AND r.unit_number = '101'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("resident of 'default': %d (%v)", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM expenses e JOIN condominiums c ON c.id = e.condominium_id WHERE c.name = 'default'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("expense of 'default': %d (%v)", n, err)
	}
}
