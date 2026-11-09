package expense_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestPostgresRepository_Expense(t *testing.T) {
	repo := expense.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	day := func(d int) time.Time { return time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC) }
	// Saved out of order to check the due-date ordering.
	for _, e := range []*expense.Expense{
		{Description: "água", AmountCents: 300, DueDate: day(20), ShopOwnerID: "shop-1"},
		{Description: "taxa", AmountCents: 15000, DueDate: day(10), ResidentID: "res-1"},
		{Description: "isenta", AmountCents: 0, DueDate: day(30), ResidentID: "res-2"},
	} {
		if err := repo.Save(ctxA, e); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
	if err := repo.Save(ctxB, &expense.Expense{Description: "outro", AmountCents: 1, DueDate: day(15), ResidentID: "r"}); err != nil {
		t.Fatal(err)
	}

	t.Run("list ordered by due date, scoped by tenant", func(t *testing.T) {
		list, err := repo.FindAll(ctxA, expense.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 3 || list[0].Description != "taxa" || list[1].Description != "água" || list[2].Description != "isenta" {
			t.Fatalf("unexpected order: %+v", list)
		}
		if list[0].ResidentID != "res-1" || list[0].ShopOwnerID != "" || list[1].ShopOwnerID != "shop-1" {
			t.Errorf("unit links not preserved: %+v %+v", list[0], list[1])
		}
		if !list[0].DueDate.Equal(day(10)) || list[0].DueDate.Location() != time.UTC {
			t.Errorf("due date must round-trip in UTC: %v", list[0].DueDate)
		}
	})

	t.Run("date filter is inclusive", func(t *testing.T) {
		from, to := day(10), day(20)
		list, err := repo.FindAll(ctxA, expense.Filter{From: &from, To: &to})
		if err != nil || len(list) != 2 {
			t.Fatalf("expected 2, got %d (%v)", len(list), err)
		}
		list, _ = repo.FindAll(ctxA, expense.Filter{From: &to})
		if len(list) != 2 {
			t.Errorf("from only: expected 2, got %d", len(list))
		}
		list, _ = repo.FindAll(ctxA, expense.Filter{To: &from})
		if len(list) != 1 {
			t.Errorf("to only: expected 1, got %d", len(list))
		}
	})

	t.Run("update, isolation, delete", func(t *testing.T) {
		list, _ := repo.FindAll(ctxA, expense.Filter{})
		e := list[0]
		e.AmountCents, e.Description = 20000, "taxa reajustada"
		if err := repo.Update(ctxA, e); err != nil {
			t.Fatal(err)
		}
		got, err := repo.FindByID(ctxA, e.ID)
		if err != nil || *got != *e {
			t.Fatalf("update not persisted: %+v (%v)", got, err)
		}
		if _, err := repo.FindByID(ctxB, e.ID); !errors.Is(err, expense.ErrNotFound) {
			t.Errorf("other tenant: expected ErrNotFound, got %v", err)
		}
		if err := repo.Update(ctxB, e); !errors.Is(err, expense.ErrNotFound) {
			t.Errorf("other tenant Update: expected ErrNotFound, got %v", err)
		}
		if err := repo.Delete(ctxB, e.ID); !errors.Is(err, expense.ErrNotFound) {
			t.Errorf("other tenant Delete: expected ErrNotFound, got %v", err)
		}
		if err := repo.Delete(ctxA, e.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.FindByID(ctxA, e.ID); !errors.Is(err, expense.ErrNotFound) {
			t.Errorf("after delete: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("negative amount rejected by the database", func(t *testing.T) {
		if err := repo.Save(ctxA, &expense.Expense{Description: "x", AmountCents: -1, DueDate: day(1), ResidentID: "r"}); err == nil {
			t.Error("expected check constraint violation")
		}
	})

	if _, err := repo.FindAll(context.Background(), expense.Filter{}); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant: expected ErrMissing, got %v", err)
	}
}
