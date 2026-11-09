package shopowner_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/shopowner"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
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
	if _, err := repo.FindByID(ctxA, "not-a-uuid"); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("malformed id: expected ErrNotFound, got %v", err)
	}

	if _, err := repo.FindByID(ctxB, s.ID); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("other tenant FindByID: expected ErrNotFound, got %v", err)
	}
	if list, total, err := repo.FindAll(ctxB, "", page.Default); err != nil || len(list) != 0 || total != 0 {
		t.Errorf("other tenant FindAll: expected empty, got %d/%d (%v)", len(list), total, err)
	}
	if ids, err := repo.IDsByOwner(ctxB, "owner-1"); err != nil || len(ids) != 0 {
		t.Errorf("other tenant IDsByOwner: expected empty, got %v (%v)", ids, err)
	}
	if err := repo.Update(ctxB, got); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("other tenant Update: expected ErrNotFound, got %v", err)
	}
	if err := repo.Delete(ctxB, s.ID); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("other tenant Delete: expected ErrNotFound, got %v", err)
	}
	if again, _ := repo.FindByID(ctxA, s.ID); again == nil || again.ShopName != "Padaria Nova" {
		t.Errorf("other tenant changed the record: %+v", again)
	}
	if err := repo.Save(ctxB, &shopowner.ShopOwner{ShopName: "Mesma", CNPJ: s.CNPJ}); err != nil {
		t.Errorf("same CNPJ in another condominium: %v", err)
	}
	if _, _, err := repo.FindAll(context.Background(), "", page.Default); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant FindAll: expected ErrMissing, got %v", err)
	}
	if _, err := repo.IDsByOwner(context.Background(), "owner-1"); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant IDsByOwner: expected ErrMissing, got %v", err)
	}
	if _, err := repo.FindByID(context.Background(), s.ID); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("missing tenant FindByID: expected ErrMissing, got %v", err)
	}

	if err := repo.Delete(ctxA, s.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctxA, s.ID); !errors.Is(err, shopowner.ErrNotFound) {
		t.Errorf("second delete: expected ErrNotFound, got %v", err)
	}
}

func TestPostgresRepository_ShopOwner_FindAll_OwnerFilterAndPaging(t *testing.T) {
	repo := shopowner.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	shops := []*shopowner.ShopOwner{
		{ShopName: "A1", CNPJ: "11.222.333/0001-81", OwnerID: "owner-a"},
		{ShopName: "B1", CNPJ: "45.997.418/0001-53", OwnerID: "owner-b"},
		{ShopName: "A2", CNPJ: "11.444.777/0001-61", OwnerID: "owner-a"},
	}
	for _, s := range shops {
		if err := repo.Save(ctxA, s); err != nil {
			t.Fatalf("save %s: %v", s.ShopName, err)
		}
	}
	// Same owner ID in another condominium must not leak into condo-a results.
	if err := repo.Save(ctxB, &shopowner.ShopOwner{ShopName: "A in B", CNPJ: "11.222.333/0001-81", OwnerID: "owner-a"}); err != nil {
		t.Fatal(err)
	}

	names := func(list []*shopowner.ShopOwner) []string {
		out := make([]string, len(list))
		for i, s := range list {
			out[i] = s.ShopName
		}
		return out
	}

	cases := []struct {
		owner string
		p     page.Params
		want  []string
		total int64
	}{
		{"", page.Default, []string{"A1", "B1", "A2"}, 3},
		{"", page.Params{Page: 1, Size: 2}, []string{"A1", "B1"}, 3},
		{"", page.Params{Page: 2, Size: 2}, []string{"A2"}, 3},
		{"", page.Params{Page: 3, Size: 2}, []string{}, 3},
		{"owner-a", page.Default, []string{"A1", "A2"}, 2},
		{"owner-a", page.Params{Page: 2, Size: 1}, []string{"A2"}, 2},
		{"owner-b", page.Default, []string{"B1"}, 1},
		{"nobody", page.Default, []string{}, 0},
	}
	for _, tc := range cases {
		list, total, err := repo.FindAll(ctxA, tc.owner, tc.p)
		if err != nil {
			t.Fatalf("FindAll(%q, %+v): %v", tc.owner, tc.p, err)
		}
		if total != tc.total || !slices.Equal(names(list), tc.want) {
			t.Errorf("FindAll(%q, %+v): got %v total %d, want %v total %d", tc.owner, tc.p, names(list), total, tc.want, tc.total)
		}
	}

	ids, err := repo.IDsByOwner(ctxA, "owner-a")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)
	want := []string{shops[0].ID, shops[2].ID}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("IDsByOwner(owner-a): got %v, want %v", ids, want)
	}
	if ids, err := repo.IDsByOwner(ctxA, "nobody"); err != nil || len(ids) != 0 {
		t.Errorf("IDsByOwner(nobody): got %v (%v)", ids, err)
	}
	if ids, err := repo.IDsByOwner(ctxB, "owner-a"); err != nil || len(ids) != 1 || slices.Contains(want, ids[0]) {
		t.Errorf("IDsByOwner in condo-b: got %v (%v)", ids, err)
	}
}
