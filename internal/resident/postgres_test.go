package resident_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/resident"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
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
		if list, total, _ := repo.FindAll(ctxB, "", page.Default); len(list) != 0 || total != 0 {
			t.Errorf("FindAll: expected empty, got %d (total %d)", len(list), total)
		}
		if ids, err := repo.IDsByOwner(ctxB, "owner-1"); err != nil || len(ids) != 0 {
			t.Errorf("IDsByOwner: expected empty, got %v (%v)", ids, err)
		}
		if ok, _ := repo.ExistsUnit(ctxB, "bloco-1", "102", ""); ok {
			t.Error("ExistsUnit: another tenant must not see the unit")
		}
		if err := repo.Update(ctxB, &resident.Resident{ID: r.ID, UnitNumber: "z", CondominiumID: "z"}); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("Update: expected ErrNotFound, got %v", err)
		}
		if err := repo.Delete(ctxB, r.ID); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("Delete: expected ErrNotFound, got %v", err)
		}
		if _, _, err := repo.FindAll(context.Background(), "", page.Default); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("missing tenant: expected ErrMissing, got %v", err)
		}
		if _, err := repo.IDsByOwner(context.Background(), "owner-1"); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("IDsByOwner missing tenant: expected ErrMissing, got %v", err)
		}
		if _, err := repo.FindByID(context.Background(), r.ID); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("FindByID missing tenant: expected ErrMissing, got %v", err)
		}
	})

	t.Run("list ordered by creation", func(t *testing.T) {
		list, total, err := repo.FindAll(ctxA, "", page.Default)
		if err != nil || len(list) != 2 || total != 2 || list[0].ID != r.ID {
			t.Errorf("unexpected list: %v total %d (%v)", list, total, err)
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

func TestPostgresRepository_Resident_FindAllOwnerFilterAndPaging(t *testing.T) {
	repo := resident.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	save := func(ctx context.Context, unit, owner string) string {
		t.Helper()
		r := &resident.Resident{UnitNumber: unit, Floor: 1, CondominiumID: "bloco-1", OwnerID: owner}
		if err := repo.Save(ctx, r); err != nil {
			t.Fatalf("save %s: %v", unit, err)
		}
		return r.ID
	}
	// Interleave owners; insertion order is the expected listing order.
	all, ownedByA := make([]string, 0, 5), make([]string, 0, 3)
	for i, owner := range []string{"owner-a", "owner-b", "owner-a", "owner-b", "owner-a"} {
		id := save(ctxA, fmt.Sprintf("%d", 101+i), owner)
		all = append(all, id)
		if owner == "owner-a" {
			ownedByA = append(ownedByA, id)
		}
	}
	// A unit of owner-a in another tenant must never show up in condo-a.
	foreign := save(ctxB, "101", "owner-a")

	ids := func(list []*resident.Resident) []string {
		out := make([]string, len(list))
		for i, r := range list {
			out[i] = r.ID
		}
		return out
	}

	t.Run("all owners, paged", func(t *testing.T) {
		cases := []struct {
			p    page.Params
			want []string
		}{
			{page.Params{Page: 1, Size: 2}, all[0:2]},
			{page.Params{Page: 2, Size: 2}, all[2:4]},
			{page.Params{Page: 3, Size: 2}, all[4:5]},
			{page.Params{Page: 4, Size: 2}, nil},
			{page.Default, all},
		}
		for _, c := range cases {
			list, total, err := repo.FindAll(ctxA, "", c.p)
			if err != nil {
				t.Fatal(err)
			}
			if total != 5 {
				t.Errorf("%+v: expected total 5, got %d", c.p, total)
			}
			if got := ids(list); !slices.Equal(got, c.want) && (len(got) != 0 || len(c.want) != 0) {
				t.Errorf("%+v: expected %v, got %v", c.p, c.want, got)
			}
		}
	})

	t.Run("owner filter, paged", func(t *testing.T) {
		list, total, err := repo.FindAll(ctxA, "owner-a", page.Params{Page: 1, Size: 2})
		if err != nil {
			t.Fatal(err)
		}
		if total != 3 || !slices.Equal(ids(list), ownedByA[0:2]) {
			t.Errorf("page 1: expected %v total 3, got %v total %d", ownedByA[0:2], ids(list), total)
		}
		list, total, _ = repo.FindAll(ctxA, "owner-a", page.Params{Page: 2, Size: 2})
		if total != 3 || !slices.Equal(ids(list), ownedByA[2:3]) {
			t.Errorf("page 2: expected %v total 3, got %v total %d", ownedByA[2:3], ids(list), total)
		}
		for _, r := range list {
			if r.OwnerID != "owner-a" {
				t.Errorf("owner filter leaked %+v", r)
			}
		}
		if list, total, _ := repo.FindAll(ctxA, "nobody", page.Default); len(list) != 0 || total != 0 {
			t.Errorf("unknown owner: expected empty, got %v total %d", ids(list), total)
		}
	})

	t.Run("IDsByOwner", func(t *testing.T) {
		got, err := repo.IDsByOwner(ctxA, "owner-a")
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		want := slices.Clone(ownedByA)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		if slices.Contains(got, foreign) {
			t.Error("IDsByOwner leaked a unit of another tenant")
		}
		if got, _ := repo.IDsByOwner(ctxA, "nobody"); len(got) != 0 {
			t.Errorf("unknown owner: expected none, got %v", got)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		list, total, err := repo.FindAll(ctxB, "", page.Default)
		if err != nil || total != 1 || len(list) != 1 || list[0].ID != foreign {
			t.Errorf("condo-b: expected only its unit, got %v total %d (%v)", ids(list), total, err)
		}
		if got, _ := repo.IDsByOwner(ctxB, "owner-a"); !slices.Equal(got, []string{foreign}) {
			t.Errorf("condo-b IDsByOwner: expected [%s], got %v", foreign, got)
		}
		if _, err := repo.FindByID(ctxA, foreign); !errors.Is(err, resident.ErrNotFound) {
			t.Errorf("condo-a FindByID of condo-b unit: expected ErrNotFound, got %v", err)
		}
	})
}
