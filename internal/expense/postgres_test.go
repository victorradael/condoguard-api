package expense_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

var all = authz.Scope{All: true}

// findAll lists the first (large) page.
func findAll(t *testing.T, repo expense.Repository, ctx context.Context, f expense.Filter) []*expense.Expense {
	t.Helper()
	list, _, err := repo.FindAll(ctx, f, page.Params{Page: 1, Size: page.MaxSize})
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	return list
}

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
		list := findAll(t, repo, ctxA, expense.Filter{Scope: all})
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
		list, total, err := repo.FindAll(ctxA, expense.Filter{From: &from, To: &to, Scope: all}, page.Default)
		if err != nil || len(list) != 2 || total != 2 {
			t.Fatalf("expected 2, got %d/%d (%v)", len(list), total, err)
		}
		list = findAll(t, repo, ctxA, expense.Filter{From: &to, Scope: all})
		if len(list) != 2 {
			t.Errorf("from only: expected 2, got %d", len(list))
		}
		list = findAll(t, repo, ctxA, expense.Filter{To: &from, Scope: all})
		if len(list) != 1 {
			t.Errorf("to only: expected 1, got %d", len(list))
		}
	})

	t.Run("scope restricts to the given units", func(t *testing.T) {
		descs := func(f expense.Filter) []string {
			list := findAll(t, repo, ctxA, f)
			out := make([]string, 0, len(list))
			for _, e := range list {
				out = append(out, e.Description)
			}
			return out
		}
		cases := []struct {
			name  string
			scope authz.Scope
			want  []string
		}{
			{"resident only", authz.Scope{Units: authz.Units{ResidentIDs: []string{"res-1"}}}, []string{"taxa"}},
			{"shop only", authz.Scope{Units: authz.Units{ShopOwnerIDs: []string{"shop-1"}}}, []string{"água"}},
			{"resident and shop", authz.Scope{Units: authz.Units{ResidentIDs: []string{"res-2"}, ShopOwnerIDs: []string{"shop-1"}}}, []string{"água", "isenta"}},
			{"several residents", authz.Scope{Units: authz.Units{ResidentIDs: []string{"res-1", "res-2"}}}, []string{"taxa", "isenta"}},
			// "r" belongs to condo-b: the scope never crosses the tenant.
			{"other tenant's unit", authz.Scope{Units: authz.Units{ResidentIDs: []string{"r"}}}, nil},
			// A shop ID is not matched against resident_id (and vice versa).
			{"ID in the wrong column", authz.Scope{Units: authz.Units{ResidentIDs: []string{"shop-1"}, ShopOwnerIDs: []string{"res-1"}}}, nil},
			{"empty units", authz.Scope{}, nil},
			{"empty slices", authz.Scope{Units: authz.Units{ResidentIDs: []string{}, ShopOwnerIDs: []string{}}}, nil},
		}
		for _, c := range cases {
			got := descs(expense.Filter{Scope: c.scope})
			if !slices.Equal(got, c.want) {
				t.Errorf("%s: expected %v, got %v", c.name, c.want, got)
			}
		}
		_, total, err := repo.FindAll(ctxA, expense.Filter{}, page.Default)
		if err != nil || total != 0 {
			t.Errorf("empty units total: expected 0, got %d (%v)", total, err)
		}
		from := day(15)
		if got := descs(expense.Filter{From: &from, Scope: authz.Scope{Units: authz.Units{ResidentIDs: []string{"res-1", "res-2"}}}}); !slices.Equal(got, []string{"isenta"}) {
			t.Errorf("scope + date filter: expected [isenta], got %v", got)
		}
	})

	t.Run("paging and total", func(t *testing.T) {
		want := []string{"taxa", "água", "isenta"}
		for i := range 3 {
			list, total, err := repo.FindAll(ctxA, expense.Filter{Scope: all}, page.Params{Page: i + 1, Size: 1})
			if err != nil || total != 3 || len(list) != 1 || list[0].Description != want[i] {
				t.Fatalf("page %d: got %+v total %d (%v)", i+1, list, total, err)
			}
		}
		list, total, err := repo.FindAll(ctxA, expense.Filter{Scope: all}, page.Params{Page: 2, Size: 2})
		if err != nil || total != 3 || len(list) != 1 || list[0].Description != "isenta" {
			t.Fatalf("page 2 of 2: got %+v total %d (%v)", list, total, err)
		}
		list, total, _ = repo.FindAll(ctxA, expense.Filter{Scope: all}, page.Params{Page: 4, Size: 1})
		if len(list) != 0 || total != 3 {
			t.Errorf("past the end: expected none with total 3, got %d/%d", len(list), total)
		}
		restricted := authz.Scope{Units: authz.Units{ResidentIDs: []string{"res-1", "res-2"}}}
		list, total, _ = repo.FindAll(ctxA, expense.Filter{Scope: restricted}, page.Params{Page: 1, Size: 1})
		if len(list) != 1 || total != 2 || list[0].Description != "taxa" {
			t.Errorf("restricted page: expected [taxa] of 2, got %+v total %d", list, total)
		}
	})

	t.Run("tenant isolation of the list", func(t *testing.T) {
		list, total, err := repo.FindAll(ctxB, expense.Filter{Scope: all}, page.Default)
		if err != nil || total != 1 || len(list) != 1 || list[0].Description != "outro" {
			t.Fatalf("condo-b: expected only its own expense, got %+v total %d (%v)", list, total, err)
		}
		if list := findAll(t, repo, tenant.WithCondominiumID(context.Background(), "condo-c"), expense.Filter{Scope: all}); len(list) != 0 {
			t.Errorf("condo-c: expected nothing, got %+v", list)
		}
	})

	t.Run("update, isolation, delete", func(t *testing.T) {
		list := findAll(t, repo, ctxA, expense.Filter{Scope: all})
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

	if _, _, err := repo.FindAll(context.Background(), expense.Filter{Scope: all}, page.Default); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant: expected ErrMissing, got %v", err)
	}
}
