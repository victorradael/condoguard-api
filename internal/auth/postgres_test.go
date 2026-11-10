package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/auth"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestPostgresRepository_Auth(t *testing.T) {
	env := dbtest.NewEnv(t)
	repo := auth.NewPostgresRepository(env.Pool)
	ctxA := tenant.WithCondominiumID(context.Background(), env.Condo("condo-a"))
	ctxB := tenant.WithCondominiumID(context.Background(), env.Condo("condo-b"))

	u := &auth.User{Username: "bia", Email: "bia@exemplo.test", Password: "hash", Roles: []string{"morador"}}
	if err := repo.Save(ctxA, u); err != nil {
		t.Fatalf("save: %v", err)
	}
	// E-mail is unique across the whole database, not only per condominium.
	for _, ctx := range []context.Context{ctxA, ctxB} {
		if err := repo.Save(ctx, &auth.User{Username: "bia2", Email: "bia@exemplo.test", Password: "h", Roles: []string{"morador"}}); !errors.Is(err, auth.ErrDuplicateEmail) {
			t.Errorf("expected ErrDuplicateEmail, got %v", err)
		}
	}
	// Usernames may repeat: they are display names now.
	if err := repo.Save(ctxA, &auth.User{Username: "bia", Email: "bia-2@exemplo.test", Password: "h", Roles: []string{"morador"}}); err != nil {
		t.Errorf("duplicate username must be accepted: %v", err)
	}

	// Login happens before the condominium is known: the account is found in the
	// whole base, with or without a tenant, and its memberships say where it belongs.
	for name, ctx := range map[string]context.Context{"condo-a": ctxA, "condo-b": ctxB, "no tenant": context.Background()} {
		got, err := repo.FindByEmail(ctx, "bia@exemplo.test")
		if err != nil || got.ID != u.ID || got.Password != "hash" {
			t.Fatalf("FindByEmail (%s): %+v, %v", name, got, err)
		}
	}
	if _, err := repo.FindByEmail(ctxA, "nobody@exemplo.test"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("unknown e-mail: expected ErrNotFound, got %v", err)
	}
	ms, err := repo.Memberships(context.Background(), u.ID)
	if err != nil || len(ms) != 1 || ms[0].CondominiumID != u.CondominiumID || len(ms[0].Roles) != 1 || ms[0].Roles[0] != "morador" {
		t.Fatalf("Memberships: %+v, %v", ms, err)
	}
	if got, err := repo.FindByID(ctxA, u.ID); err != nil || got.Email != u.Email || got.CondominiumID != u.CondominiumID || got.Roles[0] != "morador" {
		t.Fatalf("FindByID: %+v, %v", got, err)
	}
	if _, err := repo.FindByID(ctxB, u.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("other condominium: expected ErrNotFound, got %v", err)
	}
	if _, err := repo.FindByID(context.Background(), u.ID); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant: expected ErrMissing, got %v", err)
	}
}

// The database itself rejects e-mails that are not normalised.
func TestPostgres_EmailMustBeNormalised(t *testing.T) {
	env := dbtest.NewEnv(t)
	repo := auth.NewPostgresRepository(env.Pool)
	ctx := tenant.WithCondominiumID(context.Background(), env.Condo("condo-a"))
	if err := repo.Save(ctx, &auth.User{Username: "x", Email: "Upper@Exemplo.test", Password: "h", Roles: []string{"morador"}}); err == nil {
		t.Error("expected the CHECK constraint to reject a mixed-case e-mail")
	}
}

func TestPostgresTokenStore(t *testing.T) {
	env := dbtest.NewEnv(t)
	pool := env.Pool
	repo, store := auth.NewPostgresRepository(pool), auth.NewPostgresTokenStore(pool)
	ctxA := tenant.WithCondominiumID(context.Background(), env.Condo("condo-a"))
	ctxB := tenant.WithCondominiumID(context.Background(), env.Condo("condo-b"))

	u := &auth.User{Username: "c", Email: "c@exemplo.test", Password: "h", Roles: []string{"morador"}}
	if err := repo.Save(ctxA, u); err != nil {
		t.Fatal(err)
	}
	family := uuid.NewString()
	sess := auth.Session{UserID: u.ID, FamilyID: family}
	h1, h2, expired := []byte("hash-1-------------------------0"), []byte("hash-2-------------------------0"), []byte("hash-x-------------------------0")
	for hash, exp := range map[string]time.Time{string(h1): time.Now().Add(time.Hour), string(h2): time.Now().Add(time.Hour), string(expired): time.Now().Add(-time.Minute)} {
		if err := store.Insert(ctxA, sess, []byte(hash), exp); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := store.Consume(ctxB, h1); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("other condominium: expected ErrTokenNotFound, got %v", err)
	}
	if _, err := store.Consume(ctxA, expired); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("expired: expected ErrTokenNotFound, got %v", err)
	}
	got, err := store.Consume(ctxA, h1)
	if err != nil || got != sess {
		t.Fatalf("consume: %+v, %v", got, err)
	}
	if _, err := store.Consume(ctxA, h1); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("second consume: expected ErrTokenNotFound, got %v", err)
	}
	if f, err := store.Family(ctxA, h1); err != nil || f != family {
		t.Errorf("family of a consumed token: %q, %v", f, err)
	}
	if err := store.RevokeFamily(ctxA, family); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(ctxA, h2); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("after family revocation: expected ErrTokenNotFound, got %v", err)
	}
}
