package seed

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/pkg/password"
)

// ErrExists is returned by Load when a condominium with the demo name exists.
var ErrExists = errors.New("seed: the demonstration condominium already exists")

// NewPassword returns a random password that meets the policy. The seed never
// has a built-in password: it is SEED_PASSWORD or one generated per run.
func NewPassword() (string, error) {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "Demo-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)) + "9", nil
}

// Load writes the data set in one transaction as the application role (the one
// Row-Level Security applies to), setting the tenant to the new condominium.
// pw is the password of the test accounts. ErrExists when a condominium with
// the demo name is already there; nothing is written then.
func Load(ctx context.Context, pool *pgxpool.Pool, ds *Dataset, pw string) error {
	if err := password.Validate(pw); err != nil {
		return fmt.Errorf("seed: password of the test accounts: %w", err)
	}
	var existing *string
	if err := pool.QueryRow(ctx, `SELECT condominium_lookup($1)::text`, ds.Condominium).Scan(&existing); err != nil {
		return err
	}
	if existing != nil {
		return ErrExists
	}
	hashes := make(map[string]string, len(ds.Accounts))
	for _, a := range ds.Accounts {
		h, err := password.Hash(pw)
		if err != nil {
			return err
		}
		hashes[a.Email] = h
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = tx.Exec(ctx, `SELECT set_config('app.condominium_id', $1, true)`, ds.CondominiumID.String()); err != nil {
		return err
	}

	b := &pgx.Batch{}
	at := func(i int) time.Time { return baseTime.Add(-24 * time.Hour).Add(time.Duration(i) * time.Second) }
	b.Queue(`INSERT INTO administrators (id, name, created_at, is_demo) VALUES ($1, $2, $3, true)`, ds.AdministratorID, ds.Administrator, at(0))
	b.Queue(`INSERT INTO condominiums (id, administrator_id, name, created_at, is_demo) VALUES ($1, $2, $3, $4, true)`,
		ds.CondominiumID, ds.AdministratorID, ds.Condominium, at(1))
	for i, bl := range ds.Blocks {
		b.Queue(`INSERT INTO blocks (id, condominium_id, name, created_at, is_demo) VALUES ($1, $2, $3, $4, true)`, bl.ID, ds.CondominiumID, bl.Name, at(10+i))
	}
	for _, u := range ds.Units {
		b.Queue(`INSERT INTO residents (id, condominium_id, condominium_ref, unit_number, floor, owner_id, block_id, created_at, is_demo)
		         VALUES ($1, $2, $3, $4, $5, '', $6, $7, true)`,
			u.ID, ds.CondominiumID, ds.CondominiumID.String(), u.Number, u.Floor, ds.Blocks[u.Block].ID, u.CreatedAt)
	}
	userIDs := map[string]string{}
	for i, a := range ds.Accounts {
		uid := newID(ds, "user", i)
		userIDs[a.Email] = uid
		b.Queue(`INSERT INTO users (id, username, email, password_hash, email_verified_at, created_at, is_demo)
		         VALUES ($1, $2, $3, $4, $5, $5, true)`, uid, a.Username, a.Email, hashes[a.Email], at(100+i))
		b.Queue(`INSERT INTO memberships (user_id, condominium_id, roles, created_at, is_demo) VALUES ($1, $2, $3, $4, true)`,
			uid, ds.CondominiumID, []string{a.Role}, at(100+i))
	}
	accountOf := map[int]string{}
	for _, a := range ds.Accounts {
		if a.Person >= 0 {
			accountOf[a.Person] = userIDs[a.Email]
		}
	}
	for i, p := range ds.People {
		var uid *string
		if v, ok := accountOf[i]; ok {
			uid = &v
		}
		b.Queue(`INSERT INTO people (id, condominium_id, full_name, cpf, email, phone, user_id, created_at, updated_at, is_demo)
		         VALUES ($1, $2, $3, $4, $5, $6, $7::uuid, $8, $8, true)`,
			p.ID, ds.CondominiumID, p.Name, nullable(p.CPF), nullable(p.Email), nullable(p.Phone), uid, p.CreatedAt)
	}
	for _, l := range ds.Links {
		b.Queue(`INSERT INTO unit_links (id, condominium_id, unit_id, person_id, kind, starts_on, ends_on, created_at, is_demo)
		         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true)`,
			l.ID, ds.CondominiumID, ds.Units[l.Unit].ID, ds.People[l.Person].ID, l.Kind, l.Starts, l.Ends, ds.People[l.Person].CreatedAt)
	}
	res := tx.SendBatch(ctx, b)
	for i := 0; i < b.Len(); i++ {
		if _, err = res.Exec(); err != nil {
			_ = res.Close()
			return fmt.Errorf("seed: statement %d: %w", i, err)
		}
	}
	if err = res.Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func newID(ds *Dataset, kind string, n int) string {
	g := &gen{seed: ds.Seed}
	return g.id(kind, n).String()
}

// Reset deletes every demonstration condominium and what hangs from it:
// its audit trail, its data, the accounts that only belong to demonstration
// condominiums and the administradoras left without a condominium. Real
// condominiums, and accounts that also belong to one, are not touched.
// pool must connect as the schema owner (the application role is bound by
// Row-Level Security and cannot see other tenants).
func Reset(ctx context.Context, pool *pgxpool.Pool) (Removed, error) {
	var r Removed
	tx, err := pool.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	// Lets the audit trigger delete the entries of demonstration condominiums (and only those).
	if _, err = tx.Exec(ctx, `SELECT set_config('app.demo_reset', 'on', true)`); err != nil {
		return r, err
	}
	steps := []struct {
		what *int64
		sql  string
	}{
		{&r.AuditEntries, `DELETE FROM audit_log WHERE is_demo`},
		{&r.Accounts, `DELETE FROM users u WHERE u.is_demo
		                 OR (EXISTS (SELECT 1 FROM memberships m JOIN condominiums c ON c.id = m.condominium_id WHERE m.user_id = u.id AND c.is_demo)
		                     AND NOT EXISTS (SELECT 1 FROM memberships m JOIN condominiums c ON c.id = m.condominium_id WHERE m.user_id = u.id AND NOT c.is_demo))`},
		{&r.Condominiums, `DELETE FROM condominiums WHERE is_demo`},
		{&r.Administrators, `DELETE FROM administrators a WHERE a.is_demo AND NOT EXISTS (SELECT 1 FROM condominiums c WHERE c.administrator_id = a.id)`},
	}
	for _, s := range steps {
		tag, xerr := tx.Exec(ctx, s.sql)
		if xerr != nil {
			return r, fmt.Errorf("seed: reset: %w", xerr)
		}
		*s.what = tag.RowsAffected()
	}
	return r, tx.Commit(ctx)
}

// Removed says what Reset deleted.
type Removed struct {
	AuditEntries, Accounts, Condominiums, Administrators int64
}
