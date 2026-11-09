package db_test

import (
	"context"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
)

var tables = []string{"users", "residents", "shop_owners", "expenses", "notifications", "refresh_tokens", "notification_reads"}

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

// Every table must carry a NOT NULL condominium_id (tenant) column.
func TestSchema_EveryTableHasRequiredCondominiumID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.NewPool(t)

	rows, err := pool.Query(ctx, `
		SELECT t.table_name, c.is_nullable
		FROM information_schema.tables t
		LEFT JOIN information_schema.columns c
		  ON c.table_schema = t.table_schema AND c.table_name = t.table_name AND c.column_name = 'condominium_id'
		WHERE t.table_schema = 'public' AND t.table_type = 'BASE TABLE' AND t.table_name <> 'goose_db_version'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var table string
		var nullable *string
		if err := rows.Scan(&table, &nullable); err != nil {
			t.Fatal(err)
		}
		seen++
		if nullable == nil {
			t.Errorf("table %s has no condominium_id column", table)
		} else if *nullable != "NO" {
			t.Errorf("table %s: condominium_id must be NOT NULL", table)
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
