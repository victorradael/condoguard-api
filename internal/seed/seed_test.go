package seed_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/people"
	"github.com/victorradael/condoguard/api/internal/seed"
)

func TestGenerate_SizesOfThePoCProfile(t *testing.T) {
	ds, err := seed.Generate(seed.Profiles["poc"], seed.DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	c := ds.Counts()
	if c.Blocks != 3 || c.Units != 120 || c.People != 300 || c.OwnerSwaps != 15 || c.TenantSwaps != 25 {
		t.Fatalf("poc size: %+v", c)
	}
	small, err := seed.Generate(seed.Profiles["seed"], seed.DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	if c := small.Counts(); c.Blocks != 1 || c.Units != 8 {
		t.Fatalf("seed size: %+v", c)
	}
}

func TestGenerate_SameSeedSameData(t *testing.T) {
	a, _ := seed.Generate(seed.Profiles["poc"], 7)
	b, _ := seed.Generate(seed.Profiles["poc"], 7)
	if !reflect.DeepEqual(a, b) {
		t.Error("the same seed must generate the same data")
	}
	c, _ := seed.Generate(seed.Profiles["poc"], 8)
	if reflect.DeepEqual(a.People, c.People) {
		t.Error("another seed must generate other data")
	}
}

func TestGenerate_DataIsFictitiousAndConsistent(t *testing.T) {
	ds, err := seed.Generate(seed.Profiles["poc"], seed.DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	cpfs, emails := map[string]bool{}, map[string]bool{}
	for _, p := range ds.People {
		if p.CPF != "" {
			if !people.ValidCPF(p.CPF) || cpfs[p.CPF] {
				t.Fatalf("CPF %q invalid or repeated", p.CPF)
			}
			cpfs[p.CPF] = true
		}
		if p.Email != "" {
			if !strings.HasSuffix(p.Email, "@exemplo.test") || emails[p.Email] {
				t.Fatalf("e-mail %q is not at exemplo.test or is repeated", p.Email)
			}
			emails[p.Email] = true
		}
		if p.Phone != "" && !strings.Contains(p.Phone[2:], "90000") {
			t.Fatalf("phone %q is not in the fictitious range", p.Phone)
		}
	}
	// Every unit has exactly one principal owner at any time (no overlap, no gap after the first).
	type span struct{ s, e time.Time }
	owners := map[int][]span{}
	for _, l := range ds.Links {
		if l.Kind == people.KindOwner {
			e := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
			if l.Ends != nil {
				e = *l.Ends
			}
			if !e.After(l.Starts) {
				t.Fatalf("link ends before it starts: %+v", l)
			}
			owners[l.Unit] = append(owners[l.Unit], span{l.Starts, e})
		}
		if l.Starts.After(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("link starts in the future: %+v", l)
		}
	}
	for u := range ds.Units {
		sp := owners[u]
		if len(sp) == 0 {
			t.Fatalf("unit %d has no owner", u)
		}
		for i := range sp {
			for j := i + 1; j < len(sp); j++ {
				if sp[i].s.Before(sp[j].e) && sp[j].s.Before(sp[i].e) {
					t.Fatalf("unit %d: two owners at once", u)
				}
			}
		}
	}
	roles := map[string]bool{}
	for _, a := range ds.Accounts {
		roles[a.Role] = true
	}
	if len(roles) != 5 {
		t.Errorf("one test account per role expected, got %v", roles)
	}
}

func TestImportCSV_Format(t *testing.T) {
	ds, _ := seed.Generate(seed.Profiles["poc"], seed.DefaultSeed)
	var buf bytes.Buffer
	if err := ds.WriteImportCSV(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), strings.Join(people.ImportHeader, ";")+"\n") {
		t.Errorf("header: %q", strings.SplitN(buf.String(), "\n", 2)[0])
	}
	var other bytes.Buffer
	_ = ds.WriteImportCSV(&other)
	if buf.String() != other.String() {
		t.Error("the spreadsheet must be reproducible")
	}
}

// fingerprint hashes the content of the demonstration tables (not the password
// hashes, which carry a random salt).
func fingerprint(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	h := sha256.New()
	for _, q := range []string{
		`SELECT row_to_json(x)::text FROM (SELECT * FROM administrators ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM condominiums ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM blocks ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM residents ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM people ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM unit_links ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT id, username, email, email_verified_at, created_at, is_demo FROM users ORDER BY id) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM memberships ORDER BY user_id) x`,
	} {
		rows, err := pool.Query(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err = rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			h.Write([]byte(s + "\n"))
		}
		rows.Close()
	}
	return hex.EncodeToString(h.Sum(nil))
}

func count(t *testing.T, pool *pgxpool.Pool, table string, where string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table+" WHERE "+where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const testPassword = "Demo-senha-de-teste-2026"

func TestLoadAndReset(t *testing.T) {
	env := dbtest.NewEnv(t)
	ctx := context.Background()
	ds, _ := seed.Generate(seed.Profiles["poc"], seed.DefaultSeed)

	// A real condominium with its own síndico, and one account that also belongs to the demo.
	realCondo := env.Condo("Condomínio Real")
	var realUser, sharedUser string
	if err := env.Admin.QueryRow(ctx, `INSERT INTO users (username, email, password_hash) VALUES ('real', 'real@exemplo.com', 'x') RETURNING id::text`).Scan(&realUser); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Admin.Exec(ctx, `INSERT INTO memberships (user_id, condominium_id, roles) VALUES ($1, $2, '{sindico}')`, realUser, realCondo); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Admin.Exec(ctx, `INSERT INTO residents (condominium_id, condominium_ref, unit_number, owner_id) VALUES ($1, 'x', '1', '')`, realCondo); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := seed.Load(ctx, env.Pool, ds, testPassword); err != nil {
		t.Fatalf("load: %v", err)
	}
	if took := time.Since(start); took > 2*time.Minute {
		t.Errorf("loading took %s, the limit is 2 minutes", took)
	}
	t.Logf("seed-poc loaded in %s", time.Since(start))

	if n := count(t, env.Admin, "blocks", "is_demo"); n != 3 {
		t.Errorf("blocks: %d", n)
	}
	if n := count(t, env.Admin, "residents", "is_demo"); n != 120 {
		t.Errorf("units: %d", n)
	}
	if n := count(t, env.Admin, "people", "is_demo"); n != 300 {
		t.Errorf("people: %d", n)
	}
	// 15 owner swaps leave 15 ended owner links; 25 tenant swaps, 25 ended tenant links.
	if n := count(t, env.Admin, "unit_links", "is_demo AND kind = 'proprietario' AND ends_on IS NOT NULL"); n != 15 {
		t.Errorf("owner swaps: %d", n)
	}
	if n := count(t, env.Admin, "unit_links", "is_demo AND kind = 'inquilino' AND ends_on IS NOT NULL"); n != 25 {
		t.Errorf("tenant swaps: %d", n)
	}
	if n := count(t, env.Admin, "users", "is_demo AND email_verified_at IS NOT NULL"); n != 6 {
		t.Errorf("test accounts: %d", n)
	}
	for _, table := range []string{"administrators", "condominiums", "users", "memberships", "residents", "blocks", "people", "unit_links"} {
		if n := count(t, env.Admin, table, "NOT is_demo"); table != "users" && table != "condominiums" && table != "residents" && table != "memberships" && n != 0 {
			t.Errorf("%s: %d rows without is_demo", table, n)
		}
	}

	// Running again does nothing.
	if err := seed.Load(ctx, env.Pool, ds, testPassword); !errors.Is(err, seed.ErrExists) {
		t.Errorf("second load: expected ErrExists, got %v", err)
	}
	// A weak password is refused before anything is written.
	if err := seed.Load(ctx, env.Pool, ds, "123"); err == nil {
		t.Error("a weak password must be refused")
	}

	// The same seed after a reset gives the same data.
	first := fingerprint(t, env.Admin)
	// An audit entry written by the API in the demo condominium must not block the reset.
	if _, err := env.Admin.Exec(ctx, `INSERT INTO audit_log (condominium_id, action, resource) VALUES ($1, 'create', 'person')`, ds.CondominiumID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, env.Admin, "audit_log", "is_demo"); n != 1 {
		t.Fatalf("the audit entry of a demo condominium must be flagged, got %d", n)
	}
	removed, err := seed.Reset(ctx, env.Admin)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if removed.Condominiums != 1 || removed.Accounts != 6 || removed.Administrators != 1 || removed.AuditEntries != 1 {
		t.Errorf("removed: %+v", removed)
	}
	// Only the demo went away.
	if n := count(t, env.Admin, "condominiums", "true"); n != 1 {
		t.Errorf("the real condominium must remain, condominiums: %d", n)
	}
	if n := count(t, env.Admin, "residents", "true"); n != 1 {
		t.Errorf("the real unit must remain, units: %d", n)
	}
	if n := count(t, env.Admin, "users", "true"); n != 1 {
		t.Errorf("the real account must remain, users: %d", n)
	}
	_ = sharedUser
	if n := count(t, env.Admin, "people", "true") + count(t, env.Admin, "unit_links", "true") + count(t, env.Admin, "blocks", "true"); n != 0 {
		t.Errorf("demo data left behind: %d rows", n)
	}

	if err := seed.Load(ctx, env.Pool, ds, testPassword); err != nil {
		t.Fatalf("load again: %v", err)
	}
	if second := fingerprint(t, env.Admin); second != first {
		t.Error("the same seed must load the same data (ids, names, CPFs, dates)")
	}
}

func TestReset_KeepsAccountsThatAlsoBelongToARealCondominium(t *testing.T) {
	env := dbtest.NewEnv(t)
	ctx := context.Background()
	ds, _ := seed.Generate(seed.Profiles["seed"], seed.DefaultSeed)
	if err := seed.Load(ctx, env.Pool, ds, testPassword); err != nil {
		t.Fatal(err)
	}
	realCondo := env.Condo("Condomínio Real")
	var síndico string
	if err := env.Admin.QueryRow(ctx, `SELECT id::text FROM users WHERE email = 'sindico@exemplo.test'`).Scan(&síndico); err != nil {
		t.Fatal(err)
	}
	// The same person is also síndico of a real condominium.
	if _, err := env.Admin.Exec(ctx, `UPDATE users SET is_demo = false WHERE id = $1`, síndico); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Admin.Exec(ctx, `INSERT INTO memberships (user_id, condominium_id, roles) VALUES ($1, $2, '{sindico}')`, síndico, realCondo); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Reset(ctx, env.Admin); err != nil {
		t.Fatal(err)
	}
	if n := count(t, env.Admin, "users", "email = 'sindico@exemplo.test'"); n != 1 {
		t.Errorf("an account of a real condominium must survive the reset, got %d", n)
	}
	if n := count(t, env.Admin, "memberships", "true"); n != 1 {
		t.Errorf("only the real membership remains, got %d", n)
	}
}
