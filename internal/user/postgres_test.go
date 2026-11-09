package user_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
	"github.com/victorradael/condoguard/api/pkg/page"
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

	t.Run("duplicate email in another condominium", func(t *testing.T) {
		err := repo.Save(ctxB, &user.User{Username: "ana", Email: "ana@exemplo.test", Password: "h"})
		if !errors.Is(err, user.ErrDuplicate) {
			t.Fatalf("e-mail must be unique across condominiums: expected ErrDuplicate, got %v", err)
		}
	})

	t.Run("non-normalised email is rejected by the database", func(t *testing.T) {
		if err := repo.Save(ctxA, &user.User{Username: "x", Email: "Mixed@Exemplo.test", Password: "h"}); err == nil {
			t.Error("expected the users_email_normalized_check to reject a non-normalised e-mail")
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
		list, total, err := repo.FindAll(ctxB, page.Default)
		if err != nil || len(list) != 0 || total != 0 {
			t.Errorf("FindAll: expected empty, got %d/total %d (%v)", len(list), total, err)
		}
		if err = repo.Update(ctxB, &user.User{ID: u.ID, Username: "x", Email: "ana@exemplo.test"}); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("Update: expected ErrNotFound, got %v", err)
		}
		if err = repo.Delete(ctxB, u.ID); !errors.Is(err, user.ErrNotFound) {
			t.Errorf("Delete: expected ErrNotFound, got %v", err)
		}
		got, err := repo.FindByID(ctxA, u.ID)
		if err != nil || got.Username != "ana-maria" {
			t.Errorf("condo-a user must be untouched: %+v (%v)", got, err)
		}
	})

	t.Run("missing tenant is rejected", func(t *testing.T) {
		if _, _, err := repo.FindAll(context.Background(), page.Default); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("expected ErrMissing, got %v", err)
		}
		if err := repo.Save(context.Background(), &user.User{Username: "z", Email: "z@z.io"}); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("expected ErrMissing, got %v", err)
		}
		if _, err := repo.FindByID(context.Background(), u.ID); !errors.Is(err, tenant.ErrMissing) {
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

func TestPostgresRepository_User_PagingAndTenantTotals(t *testing.T) {
	repo := user.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	for i := 1; i <= 5; i++ {
		if err := repo.Save(ctxA, &user.User{Username: fmt.Sprintf("a%d", i), Email: fmt.Sprintf("a%d@exemplo.test", i), Password: "h"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 2; i++ {
		if err := repo.Save(ctxB, &user.User{Username: fmt.Sprintf("b%d", i), Email: fmt.Sprintf("b%d@exemplo.test", i), Password: "h"}); err != nil {
			t.Fatal(err)
		}
	}

	all, total, err := repo.FindAll(ctxA, page.Params{Page: 1, Size: 100})
	if err != nil || total != 5 || len(all) != 5 {
		t.Fatalf("condo-a full list: %d items, total %d (%v)", len(all), total, err)
	}
	for _, u := range all {
		if u.Username[0] != 'a' {
			t.Errorf("condo-a list leaks %q", u.Username)
		}
	}

	p2, total, err := repo.FindAll(ctxA, page.Params{Page: 2, Size: 2})
	if err != nil || total != 5 || len(p2) != 2 {
		t.Fatalf("page 2: %d items, total %d (%v)", len(p2), total, err)
	}
	if p2[0].ID != all[2].ID || p2[1].ID != all[3].ID {
		t.Errorf("page 2 must be items 3-4 of the stable order")
	}

	p3, total, err := repo.FindAll(ctxA, page.Params{Page: 3, Size: 2})
	if err != nil || total != 5 || len(p3) != 1 || p3[0].ID != all[4].ID {
		t.Errorf("page 3: %d items, total %d (%v)", len(p3), total, err)
	}

	past, total, err := repo.FindAll(ctxA, page.Params{Page: 9, Size: 2})
	if err != nil || total != 5 || len(past) != 0 {
		t.Errorf("past the end: %d items, total %d (%v)", len(past), total, err)
	}

	listB, totalB, err := repo.FindAll(ctxB, page.Default)
	if err != nil || totalB != 2 || len(listB) != 2 {
		t.Errorf("condo-b: %d items, total %d (%v)", len(listB), totalB, err)
	}
}
