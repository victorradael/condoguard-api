package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/victorradael/condoguard/api/internal/auth"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestPostgresRepository_Auth(t *testing.T) {
	repo := auth.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	u := &auth.User{Username: "bia", Email: "bia@exemplo.test", Password: "hash", Roles: []string{"ROLE_USER"}}
	if err := repo.Save(ctxA, u); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := repo.Save(ctxA, &auth.User{Username: "bia2", Email: "bia@exemplo.test", Password: "h"}); !errors.Is(err, auth.ErrDuplicateEmail) {
		t.Errorf("expected ErrDuplicateEmail, got %v", err)
	}

	got, err := repo.FindByUsername(ctxA, "bia")
	if err != nil || got.ID != u.ID || got.Password != "hash" {
		t.Fatalf("FindByUsername: %+v, %v", got, err)
	}
	if got, err := repo.FindByEmail(ctxA, "bia@exemplo.test"); err != nil || got.ID != u.ID {
		t.Fatalf("FindByEmail: %+v, %v", got, err)
	}

	if _, err := repo.FindByUsername(ctxB, "bia"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("other condominium: expected ErrNotFound, got %v", err)
	}
	if _, err := repo.FindByEmail(ctxB, "bia@exemplo.test"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("other condominium: expected ErrNotFound, got %v", err)
	}
	if _, err := repo.FindByUsername(context.Background(), "bia"); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant: expected ErrMissing, got %v", err)
	}

	// Usernames are not unique; login resolves to the oldest account, deterministically.
	if err := repo.Save(ctxA, &auth.User{Username: "bia", Email: "bia-2@exemplo.test", Password: "other"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.FindByUsername(ctxA, "bia"); got.ID != u.ID {
		t.Errorf("expected the first account, got %s", got.ID)
	}
}
