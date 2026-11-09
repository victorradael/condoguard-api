package shopowner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/shopowner"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestPostgresRepository_ShopOwner(t *testing.T) {
	repo := shopowner.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	s := &shopowner.ShopOwner{ShopName: "Padaria", CNPJ: "11.222.333/0001-81", Floor: 0, OwnerID: "owner-1"}
	if err := repo.Save(ctxA, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := repo.Save(ctxA, &shopowner.ShopOwner{ShopName: "Outra", CNPJ: s.CNPJ}); !errors.Is(err, shopowner.ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}

	got, err := repo.FindByID(ctxA, s.ID)
	if err != nil || *got != *s {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	got.ShopName, got.Floor = "Padaria Nova", 2
	if err := repo.Update(ctxA, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.FindByID(ctxA, s.ID); again.ShopName != "Padaria Nova" || again.Floor != 2 {
		t.Errorf("update not persisted: %+v", again)
	}

	if _, err := repo.FindByID(ctxB, s.ID); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("other tenant FindByID: expected ErrNotFound, got %v", err)
	}
	if list, _ := repo.FindAll(ctxB); len(list) != 0 {
		t.Errorf("other tenant FindAll: expected empty, got %d", len(list))
	}
	if err := repo.Update(ctxB, got); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("other tenant Update: expected ErrNotFound, got %v", err)
	}
	if err := repo.Delete(ctxB, s.ID); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("other tenant Delete: expected ErrNotFound, got %v", err)
	}
	if err := repo.Save(ctxB, &shopowner.ShopOwner{ShopName: "Mesma", CNPJ: s.CNPJ}); err != nil {
		t.Errorf("same CNPJ in another condominium: %v", err)
	}
	if _, err := repo.FindAll(context.Background()); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant: expected ErrMissing, got %v", err)
	}

	if err := repo.Delete(ctxA, s.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctxA, s.ID); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("second delete: expected ErrNotFound, got %v", err)
	}
}
