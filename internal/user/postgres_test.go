package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
)

func TestPostgresRepository_User(t *testing.T) {
	repo := user.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	u := &user.User{Username: "ana", Email: "ana@exemplo.test", Password: "hash", Roles: []string{"ROLE_ADMIN"}}
	if err := repo.Save(ctxA, u); err != nil {
		t.Fatalf("save: %v", err)
	}
	if u.ID == "" {
		t.Fatal("save must set ID")
	}

	t.Run("duplicate email in same condominium", func(t *testing.T) {
		err := repo.Save(ctxA, &user.User{Username: "other", Email: "ana@exemplo.test", Password: "h"})
		if !errors.Is(err, user.ErrDuplicate) {
			t.Fatalf("expected ErrDuplicate, got %v", err)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		got, err := repo.FindByID(ctxA, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Username != "ana" || got.Email != "ana@exemplo.test" || got.Password != "hash" || len(got.Roles) != 1 || got.Roles[0] != "ROLE_ADMIN" {
			t.Errorf("unexpected user: %+v", got)
		}
		got.Username = "ana-maria"
		got.Roles = []string{"ROLE_USER", "ROLE_ADMIN"}
		if err := repo.Update(ctxA, got); err != nil {
			t.Fatal(err)
		}
		again, _ := repo.FindByID(ctxA, u.ID)
		if again.Username != "ana-maria" || len(again.Roles) != 2 {
			t.Errorf("update not persisted: %+v", again)
		}
	})

	t.Run("other condominium cannot see, update or delete", func(t *testing.T) {
		if _, err := repo.FindByID(ctxB, u.ID); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("FindByID: expected ErrNotFound, got %v", err)
		}
		list, err := repo.FindAll(ctxB)
		if err != nil || len(list) != 0 {
			t.Errorf("FindAll: expected empty, got %d (%v)", len(list), err)
		}
		if err := repo.Update(ctxB, &user.User{ID: u.ID, Username: "x", Email: "x"}); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("Update: expected ErrNotFound, got %v", err)
		}
		if err := repo.Delete(ctxB, u.ID); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("Delete: expected ErrNotFound, got %v", err)
		}
		// Same e-mail is allowed in another condominium.
		if err := repo.Save(ctxB, &user.User{Username: "ana", Email: "ana@exemplo.test", Password: "h"}); err != nil {
			t.Errorf("save in other condominium: %v", err)
		}
	})

	t.Run("missing tenant is rejected", func(t *testing.T) {
		if _, err := repo.FindAll(context.Background()); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("expected ErrMissing, got %v", err)
		}
		if err := repo.Save(context.Background(), &user.User{Username: "z", Email: "z@z"}); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("expected ErrMissing, got %v", err)
		}
	})

	t.Run("malformed ID is not found", func(t *testing.T) {
		if _, err := repo.FindByID(ctxA, "nonexistent"); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := repo.Delete(ctxA, u.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctxA, u.ID); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("second delete: expected ErrNotFound, got %v", err)
		}
	})
}
