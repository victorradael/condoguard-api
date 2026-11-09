package resident_test

import (
	"context"
	"errors"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/resident"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestPostgresRepository_Resident(t *testing.T) {
	repo := resident.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	r := &resident.Resident{UnitNumber: "101", Floor: 1, CondominiumID: "bloco-1", OwnerID: "owner-1"}
	if err := repo.Save(ctxA, r); err != nil {
		t.Fatalf("save: %v", err)
	}

	t.Run("unit number unique per condominiumId", func(t *testing.T) {
		err := repo.Save(ctxA, &resident.Resident{UnitNumber: "101", CondominiumID: "bloco-1", OwnerID: "o"})
		if !errors.Is(err, resident.ErrDuplicate) {
			t.Errorf("expected ErrDuplicate, got %v", err)
		}
		// Same unit number under another condominiumId value is allowed (current API behavior).
		if err := repo.Save(ctxA, &resident.Resident{UnitNumber: "101", CondominiumID: "bloco-2", OwnerID: "o"}); err != nil {
			t.Errorf("other condominiumId: %v", err)
		}
	})

	t.Run("ExistsUnit", func(t *testing.T) {
		if ok, err := repo.ExistsUnit(ctxA, "bloco-1", "101", ""); err != nil || !ok {
			t.Errorf("expected true, got %v (%v)", ok, err)
		}
		if ok, _ := repo.ExistsUnit(ctxA, "bloco-1", "101", r.ID); ok {
			t.Error("excluding itself must return false")
		}
		if ok, _ := repo.ExistsUnit(ctxA, "bloco-1", "999", ""); ok {
			t.Error("unknown unit must return false")
		}
		if ok, _ := repo.ExistsUnit(ctxB, "bloco-1", "101", ""); ok {
			t.Error("another tenant must not see the unit")
		}
	})

	t.Run("round trip and update", func(t *testing.T) {
		got, err := repo.FindByID(ctxA, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if *got != *r {
			t.Errorf("expected %+v, got %+v", r, got)
		}
		got.UnitNumber, got.Floor = "102", 3
		if err := repo.Update(ctxA, got); err != nil {
			t.Fatal(err)
		}
		again, _ := repo.FindByID(ctxA, r.ID)
		if again.UnitNumber != "102" || again.Floor != 3 || again.CondominiumID != "bloco-1" {
			t.Errorf("update not persisted: %+v", again)
		}
		// Updating into an existing unit violates the unique constraint.
		again.UnitNumber, again.CondominiumID = "101", "bloco-2"
		if err := repo.Update(ctxA, again); !errors.Is(err, resident.ErrDuplicate) {
			t.Errorf("expected ErrDuplicate, got %v", err)
		}
	})

	t.Run("floor out of range", func(t *testing.T) {
		err := repo.Save(ctxA, &resident.Resident{UnitNumber: "x", CondominiumID: "c", Floor: 1 << 40})
		if !errors.Is(err, resident.ErrValidation) {
			t.Errorf("expected ErrValidation, got %v", err)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		if _, err := repo.FindByID(ctxB, r.ID); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("FindByID: expected ErrNotFound, got %v", err)
		}
		if list, _ := repo.FindAll(ctxB); len(list) != 0 {
			t.Errorf("FindAll: expected empty, got %d", len(list))
		}
		if err := repo.Update(ctxB, &resident.Resident{ID: r.ID, UnitNumber: "z", CondominiumID: "z"}); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("Update: expected ErrNotFound, got %v", err)
		}
		if err := repo.Delete(ctxB, r.ID); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("Delete: expected ErrNotFound, got %v", err)
		}
		if _, err := repo.FindAll(context.Background()); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("missing tenant: expected ErrMissing, got %v", err)
		}
	})

	t.Run("list ordered by creation", func(t *testing.T) {
		list, err := repo.FindAll(ctxA)
		if err != nil || len(list) != 2 || list[0].ID != r.ID {
			t.Errorf("unexpected list: %v (%v)", list, err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := repo.Delete(ctxA, r.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.FindByID(ctxA, r.ID); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
