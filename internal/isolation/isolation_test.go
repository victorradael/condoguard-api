// Package isolation_test is the tenant-isolation suite of the 1b foundation.
//
// For every table a user of condominium A must not read, change or delete
// anything of condominium B. The statements here are written WITHOUT the
// "WHERE condominium_id = ..." filter the repositories add, and run as the
// restricted role the API uses in production: only PostgreSQL's Row-Level
// Security stands between A and B's rows. (The first layer, the repository
// filter, is covered by each repository's tests and by the parity suite.)
package isolation_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

type fixture struct {
	env     *dbtest.Env
	userIDs map[string]string // condominium → user ID
}

// seed puts one row of every kind in the condominium, as the schema owner.
func seed(t *testing.T, env *dbtest.Env, condo, tag string) string {
	t.Helper()
	ctx := context.Background()
	var userID string
	if err := env.Admin.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, 'hash') RETURNING id::text`,
		"user-"+tag, "user-"+tag+"@exemplo.test").Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var notifID string
	steps := []struct {
		sql  string
		args []any
		into *string
	}{
		{`INSERT INTO memberships (user_id, condominium_id, roles) VALUES ($1, $2, '{morador}')`, []any{userID, condo}, nil},
		{`INSERT INTO residents (condominium_id, condominium_ref, unit_number, owner_id) VALUES ($1, 'bloco', '101-' || $2::text, $3)`, []any{condo, tag, userID}, nil},
		{`INSERT INTO blocks (condominium_id, name) VALUES ($1, 'Bloco ' || $2::text)`, []any{condo, tag}, nil},
		{`INSERT INTO people (condominium_id, full_name, email) VALUES ($1, 'Pessoa ' || $2::text, $2::text || '@exemplo.test')`, []any{condo, tag}, nil},
		{`INSERT INTO unit_links (condominium_id, unit_id, person_id, kind, starts_on)
		  SELECT $1, r.id, p.id, 'morador', '2020-01-01' FROM residents r, people p WHERE r.condominium_id = $1 AND p.condominium_id = $1`, []any{condo}, nil},
		{`INSERT INTO invitations (condominium_id, person_id, email, token_hash, channel, expires_at)
		  SELECT $1, p.id, p.email, decode(md5($2), 'hex'), 'link', now() + interval '1 day' FROM people p WHERE p.condominium_id = $1`, []any{condo, tag}, nil},
		{`INSERT INTO shop_owners (condominium_id, shop_name, cnpj, owner_id) VALUES ($1, 'Loja', '00.000.000/0001-' || $2::text, $3)`, []any{condo, tag, userID}, nil},
		{`INSERT INTO expenses (condominium_id, description, amount_cents, due_date) VALUES ($1, 'Água', 100, now())`, []any{condo}, nil},
		{`INSERT INTO notifications (condominium_id, message, created_by_id) VALUES ($1, 'Aviso', $2) RETURNING id::text`, []any{condo, userID}, &notifID},
		{`INSERT INTO refresh_tokens (condominium_id, user_id, family_id, token_hash, expires_at) VALUES ($1, $2, gen_random_uuid(), decode(md5($3), 'hex'), now() + interval '1 day')`, []any{condo, userID, tag}, nil},
		{`INSERT INTO audit_log (condominium_id, action, resource) VALUES ($1, 'create', 'resident')`, []any{condo}, nil},
	}
	for _, s := range steps {
		var err error
		if s.into != nil {
			err = env.Admin.QueryRow(ctx, s.sql, s.args...).Scan(s.into)
		} else {
			_, err = env.Admin.Exec(ctx, s.sql, s.args...)
		}
		if err != nil {
			t.Fatalf("seed %q: %v", s.sql, err)
		}
	}
	if _, err := env.Admin.Exec(ctx, `INSERT INTO notification_reads (notification_id, user_id, condominium_id) VALUES ($1, $2, $3)`, notifID, userID, condo); err != nil {
		t.Fatalf("seed notification_reads: %v", err)
	}
	return userID
}

func setup(t *testing.T) (f *fixture, a, b string) {
	t.Helper()
	env := dbtest.NewEnv(t)
	a, b = env.Condo("A"), env.Condo("B")
	f = &fixture{env: env, userIDs: map[string]string{}}
	f.userIDs[a] = seed(t, env, a, "a")
	f.userIDs[b] = seed(t, env, b, "b")
	return f, a, b
}

// tenantTables are the tables whose rows belong to one condominium, with the
// statement that changes one of their columns.
var tenantTables = []struct{ table, update string }{
	{"memberships", `UPDATE memberships SET roles = '{sindico}'`},
	{"residents", `UPDATE residents SET floor = 99`},
	{"shop_owners", `UPDATE shop_owners SET floor = 99`},
	{"blocks", `UPDATE blocks SET name = 'hacked'`},
	{"people", `UPDATE people SET full_name = 'hacked'`},
	{"unit_links", `UPDATE unit_links SET ends_on = '2030-01-01'`},
	{"invitations", `UPDATE invitations SET revoked_at = now()`},
	{"expenses", `UPDATE expenses SET amount_cents = 999999`},
	{"notifications", `UPDATE notifications SET message = 'hacked'`},
	{"notification_reads", `UPDATE notification_reads SET read_at = now() - interval '1 year'`},
	{"refresh_tokens", `UPDATE refresh_tokens SET revoked_at = now()`},
	{"audit_log", `SELECT 1`}, // append-only: changes are covered below
}

func count(t *testing.T, pool *pgxpool.Pool, ctx context.Context, query string) int {
	t.Helper()
	var n int
	if err := db.NewScoped(pool).QueryRow(ctx, query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func adminCount(t *testing.T, f *fixture, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.env.Admin.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func asUser(f *fixture, condo string) context.Context {
	return tenant.WithUserID(tenant.WithCondominiumID(context.Background(), condo), f.userIDs[condo])
}

func TestRLS_UserOfAReadsOnlyA(t *testing.T) {
	f, a, _ := setup(t)
	ctx := asUser(f, a)
	for _, tt := range tenantTables {
		if n := count(t, f.env.Pool, ctx, "SELECT count(*) FROM "+tt.table); n != 1 {
			t.Errorf("%s: user of A sees %d rows, want only A's 1", tt.table, n)
		}
		if n := count(t, f.env.Pool, ctx, "SELECT count(*) FROM "+tt.table+" WHERE condominium_id <> '"+a+"'"); n != 0 {
			t.Errorf("%s: user of A sees %d rows of another condominium", tt.table, n)
		}
	}
	// Accounts are visible through the membership; condominiums and
	// administradoras only for the tenant itself.
	if n := count(t, f.env.Pool, ctx, "SELECT count(*) FROM users"); n != 1 {
		t.Errorf("users: user of A sees %d accounts, want 1", n)
	}
	if n := count(t, f.env.Pool, ctx, "SELECT count(*) FROM condominiums"); n != 1 {
		t.Errorf("condominiums: user of A sees %d, want 1", n)
	}
}

func TestRLS_UserOfACannotChangeB(t *testing.T) {
	f, a, b := setup(t)
	ctx := asUser(f, a)
	scoped := db.NewScoped(f.env.Pool)
	for _, tt := range tenantTables {
		if tt.update == "SELECT 1" {
			continue
		}
		// Unfiltered statement: it reaches every row the policies let through.
		tag, err := scoped.Exec(ctx, tt.update)
		if err != nil {
			t.Fatalf("%s: %v", tt.update, err)
		}
		if tag.RowsAffected() != 1 {
			t.Errorf("%s: changed %d rows, want only A's 1", tt.update, tag.RowsAffected())
		}
	}
	// B's rows are untouched.
	for _, q := range []string{
		`SELECT count(*) FROM memberships WHERE condominium_id = $1 AND roles <> '{morador}'`,
		`SELECT count(*) FROM residents WHERE condominium_id = $1 AND floor = 99`,
		`SELECT count(*) FROM shop_owners WHERE condominium_id = $1 AND floor = 99`,
		`SELECT count(*) FROM blocks WHERE condominium_id = $1 AND name = 'hacked'`,
		`SELECT count(*) FROM people WHERE condominium_id = $1 AND full_name = 'hacked'`,
		`SELECT count(*) FROM unit_links WHERE condominium_id = $1 AND ends_on IS NOT NULL`,
		`SELECT count(*) FROM invitations WHERE condominium_id = $1 AND revoked_at IS NOT NULL`,
		`SELECT count(*) FROM expenses WHERE condominium_id = $1 AND amount_cents = 999999`,
		`SELECT count(*) FROM notifications WHERE condominium_id = $1 AND message = 'hacked'`,
		`SELECT count(*) FROM notification_reads WHERE condominium_id = $1 AND read_at < now() - interval '6 months'`,
		`SELECT count(*) FROM refresh_tokens WHERE condominium_id = $1 AND revoked_at IS NOT NULL`,
	} {
		if n := adminCount(t, f, q, b); n != 0 {
			t.Errorf("B was changed by a user of A (%s): %d rows", q, n)
		}
	}
}

func TestRLS_UserOfACannotDeleteB(t *testing.T) {
	f, a, b := setup(t)
	ctx := asUser(f, a)
	scoped := db.NewScoped(f.env.Pool)
	// Children first: the foreign keys would otherwise stop the unfiltered DELETEs.
	for _, table := range []string{"notification_reads", "refresh_tokens", "notifications", "expenses", "invitations", "unit_links", "people", "residents", "blocks", "shop_owners"} {
		tag, err := scoped.Exec(ctx, "DELETE FROM "+table)
		if err != nil {
			t.Fatalf("DELETE FROM %s: %v", table, err)
		}
		if tag.RowsAffected() != 1 {
			t.Errorf("DELETE FROM %s removed %d rows, want only A's 1", table, tag.RowsAffected())
		}
		if n := adminCount(t, f, "SELECT count(*) FROM "+table+" WHERE condominium_id = $1", b); n != 1 {
			t.Errorf("%s: B has %d rows after A's unfiltered DELETE, want 1", table, n)
		}
	}
	if tag, err := scoped.Exec(ctx, "DELETE FROM users"); err != nil || tag.RowsAffected() != 0 {
		// The account of A still has a membership row, which the FK restricts.
		t.Logf("DELETE FROM users: %v rows, err %v", tag.RowsAffected(), err)
	}
	if n := adminCount(t, f, "SELECT count(*) FROM users WHERE id = $1", f.userIDs[b]); n != 1 {
		t.Errorf("the account of B was deleted by a user of A")
	}
	if n := adminCount(t, f, "SELECT count(*) FROM condominiums WHERE id = $1", b); n != 1 {
		t.Errorf("condominium B was deleted by a user of A")
	}
}

func TestRLS_UserOfACannotWriteIntoB(t *testing.T) {
	f, a, b := setup(t)
	ctx := asUser(f, a)
	scoped := db.NewScoped(f.env.Pool)
	inserts := map[string]string{
		"residents":     `INSERT INTO residents (condominium_id, condominium_ref, unit_number, owner_id) VALUES ('` + b + `', 'x', '999', 'x')`,
		"shop_owners":   `INSERT INTO shop_owners (condominium_id, shop_name, cnpj, owner_id) VALUES ('` + b + `', 'x', '1', 'x')`,
		"expenses":      `INSERT INTO expenses (condominium_id, description, amount_cents, due_date) VALUES ('` + b + `', 'x', 1, now())`,
		"notifications": `INSERT INTO notifications (condominium_id, message, created_by_id) VALUES ('` + b + `', 'x', 'x')`,
		"memberships":   `INSERT INTO memberships (user_id, condominium_id, roles) VALUES ('` + f.userIDs[a] + `', '` + b + `', '{sindico}')`,
		"blocks":        `INSERT INTO blocks (condominium_id, name) VALUES ('` + b + `', 'x')`,
		"people":        `INSERT INTO people (condominium_id, full_name) VALUES ('` + b + `', 'x')`,
		"audit_log":     `INSERT INTO audit_log (condominium_id, action, resource) VALUES ('` + b + `', 'create', 'x')`,
		"condominiums":  `INSERT INTO condominiums (name) VALUES ('sneaky')`,
	}
	for table, q := range inserts {
		if _, err := scoped.Exec(ctx, q); err == nil {
			t.Errorf("%s: a user of A inserted a row for B (or an unowned row)", table)
		}
	}
	// Moving own row into B is also refused (WITH CHECK on the new row).
	if _, err := scoped.Exec(ctx, `UPDATE residents SET condominium_id = '`+b+`'`); err == nil {
		t.Error("residents: a user of A moved a row into B")
	}
	// Neither can A give itself a role in B.
	if n := adminCount(t, f, "SELECT count(*) FROM memberships WHERE user_id = $1 AND condominium_id = $2", f.userIDs[a], b); n != 0 {
		t.Error("memberships: A's account got a membership in B")
	}
}

func TestRLS_FailsClosedWithoutATenant(t *testing.T) {
	f, a, _ := setup(t)
	contexts := map[string]context.Context{
		"no tenant":      context.Background(),
		"invalid tenant": tenant.WithCondominiumID(context.Background(), "not-a-uuid"),
		"unknown tenant": tenant.WithCondominiumID(context.Background(), "c0000000-0000-4000-8000-00000000000c"),
		"sql in tenant":  tenant.WithCondominiumID(context.Background(), a+"' OR '1'='1"),
	}
	tables := []string{"users", "condominiums", "administrators", "memberships", "residents", "shop_owners", "expenses", "notifications", "notification_reads", "refresh_tokens", "audit_log", "account_tokens", "recovery_codes", "blocks", "people", "unit_links", "invitations"}
	for name, ctx := range contexts {
		for _, table := range tables {
			if n := count(t, f.env.Pool, ctx, "SELECT count(*) FROM "+table); n != 0 {
				t.Errorf("%s: %s shows %d rows", name, table, n)
			}
		}
	}
}

// The tenant of one request never leaks into the next one that reuses the connection.
func TestRLS_TenantDoesNotLeakAcrossPooledConnections(t *testing.T) {
	f, a, b := setup(t)
	// A one-connection pool forces every statement through the same connection.
	cfg := f.env.Pool.Config()
	cfg.MaxConns = 1
	one, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	for i := 0; i < 3; i++ {
		for _, condo := range []string{a, b} {
			if n := count(t, one, asUser(f, condo), "SELECT count(*) FROM residents"); n != 1 {
				t.Fatalf("round %d, tenant %s: sees %d residents, want 1", i, condo, n)
			}
			if n := count(t, one, context.Background(), "SELECT count(*) FROM residents"); n != 0 {
				t.Fatalf("round %d: a request without tenant sees %d residents of the previous one", i, n)
			}
		}
	}
}

func TestAuditLog_IsAppendOnly(t *testing.T) {
	f, a, b := setup(t)
	ctx := asUser(f, a)
	scoped := db.NewScoped(f.env.Pool)

	// A user of A reads and writes A's trail only.
	if n := count(t, f.env.Pool, ctx, "SELECT count(*) FROM audit_log"); n != 1 {
		t.Errorf("audit_log: A sees %d entries, want 1", n)
	}
	if _, err := scoped.Exec(ctx, `INSERT INTO audit_log (condominium_id, action, resource) VALUES ('`+a+`', 'create', 'x')`); err != nil {
		t.Errorf("A cannot append to its own trail: %v", err)
	}
	// Nobody changes it: not the API role, not even the schema owner.
	for _, q := range []string{
		"UPDATE audit_log SET action = 'forged'",
		"DELETE FROM audit_log",
		"TRUNCATE audit_log",
	} {
		if _, err := scoped.Exec(ctx, q); err == nil {
			t.Errorf("the API role ran %q on the audit trail", q)
		}
		if _, err := f.env.Admin.Exec(context.Background(), q); err == nil {
			t.Errorf("the schema owner ran %q on the audit trail", q)
		}
	}
	if n := adminCount(t, f, "SELECT count(*) FROM audit_log WHERE condominium_id = $1", b); n != 1 {
		t.Errorf("B's trail changed: %d entries", n)
	}
}

// The restricted role is what the API runs as: no superuser, no BYPASSRLS, owns nothing.
func TestRLS_TheAPIRoleIsSubjectToThePolicies(t *testing.T) {
	f, _, _ := setup(t)
	var super, bypass bool
	var owned int
	err := f.env.Pool.QueryRow(context.Background(), `
		SELECT r.rolsuper, r.rolbypassrls,
		       (SELECT count(*) FROM pg_class c WHERE c.relowner = r.oid)
		FROM pg_roles r WHERE r.rolname = current_user`).Scan(&super, &bypass, &owned)
	if err != nil {
		t.Fatal(err)
	}
	if super || bypass || owned != 0 {
		t.Errorf("the API role must be a plain role that owns nothing: super=%v bypassrls=%v owns=%d", super, bypass, owned)
	}
}

// 00004: recovery codes belong to their owner (the condominium's staff may
// only delete them, when resetting a 2FA); the token table is closed to the API role.
func TestRLS_AccountSecurityTables(t *testing.T) {
	f, a, b := setup(t)
	ctx := context.Background()
	for condo, uid := range f.userIDs {
		if _, err := f.env.Admin.Exec(ctx, `INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, decode(md5($2), 'hex'))`, uid, uid); err != nil {
			t.Fatal(err)
		}
		if _, err := f.env.Admin.Exec(ctx, `INSERT INTO account_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1, 'password_reset', decode(md5($2), 'hex'), now() + interval '1 hour')`, uid, condo); err != nil {
			t.Fatal(err)
		}
	}
	scoped := db.NewScoped(f.env.Pool)

	// Another member's codes are invisible and cannot be forged or spent.
	as := asUser(f, a)
	if n := count(t, f.env.Pool, as, "SELECT count(*) FROM recovery_codes"); n != 1 {
		t.Errorf("user of A sees %d recovery codes, want only their own", n)
	}
	if _, err := scoped.Exec(as, `INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, '\x01')`, f.userIDs[b]); err == nil {
		t.Error("user of A planted a recovery code for B")
	}
	if tag, _ := scoped.Exec(as, `UPDATE recovery_codes SET used_at = now()`); tag.RowsAffected() != 1 {
		t.Errorf("update reached %d codes, want only A's 1", tag.RowsAffected())
	}
	if n := adminCount(t, f, `SELECT count(*) FROM recovery_codes WHERE used_at IS NOT NULL AND user_id = $1`, f.userIDs[b]); n != 0 {
		t.Error("B's recovery code was spent by a user of A")
	}

	// The tokens are not readable or writable by the API role at all.
	if n := count(t, f.env.Pool, as, "SELECT count(*) FROM account_tokens"); n != 0 {
		t.Errorf("API role reads %d account tokens, want none", n)
	}
	if _, err := scoped.Exec(as, `INSERT INTO account_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1, 'password_reset', '\x02', now() + interval '1 hour')`, f.userIDs[a]); err == nil {
		t.Error("API role inserted an account token directly")
	}
	if tag, _ := scoped.Exec(as, `UPDATE account_tokens SET used_at = NULL`); tag.RowsAffected() != 0 {
		t.Error("API role updated account tokens directly")
	}

	// A member of A cannot delete the codes of B; staff of A deletes only A's.
	if tag, _ := scoped.Exec(as, `DELETE FROM recovery_codes`); tag.RowsAffected() != 1 {
		t.Errorf("delete removed %d codes, want only A's 1", tag.RowsAffected())
	}
	if n := adminCount(t, f, `SELECT count(*) FROM recovery_codes WHERE user_id = $1`, f.userIDs[b]); n != 1 {
		t.Error("B's recovery code was deleted by a user of A")
	}
}
