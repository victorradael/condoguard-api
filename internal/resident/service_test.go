package resident_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/resident"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

func newService() *resident.Service {
	return resident.NewService(resident.NewInMemoryRepository())
}

func identityCtx(condoID, userID, role string) context.Context {
	ctx := tenant.WithCondominiumID(context.Background(), condoID)
	return middleware.WithIdentity(ctx, userID, []string{role})
}

// adminCtx is an admin of condo-a.
func adminCtx() context.Context { return identityCtx(condoA, "admin-1", authz.RoleAdmin) }

// userCtx is a ROLE_USER of condo-a.
func userCtx(userID string) context.Context { return identityCtx(condoA, userID, authz.RoleUser) }

func mustCreate(t *testing.T, svc *resident.Service, req resident.CreateRequest) *resident.Resident {
	t.Helper()
	r, err := svc.Create(adminCtx(), req)
	if err != nil {
		t.Fatalf("setup create: %v", err)
	}
	return r
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestResident_Create_Success(t *testing.T) {
	svc := newService()

	r, err := svc.Create(adminCtx(), resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         1,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if r.ID == "" {
		t.Error("expected non-empty ID")
	}
	if r.UnitNumber != "101" {
		t.Errorf("expected UnitNumber '101', got %q", r.UnitNumber)
	}
	if r.CondominiumID != "condo-1" {
		t.Errorf("expected CondominiumID 'condo-1', got %q", r.CondominiumID)
	}
}

func TestResident_Create_MissingUnitNumber_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), resident.CreateRequest{
		Floor:         1,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	if !resident.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestResident_Create_MissingCondominiumID_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), resident.CreateRequest{
		UnitNumber: "101",
		Floor:      1,
		OwnerID:    "user-1",
	})

	if !resident.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestResident_Create_MissingOwnerID_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         1,
		CondominiumID: "condo-1",
	})

	if !resident.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

// ── Unicidade de unidade por condomínio ───────────────────────────────────────

func TestResident_Create_DuplicateUnitInSameCondominium_ReturnsDuplicateError(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, err := svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         1,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	_, err = svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         2,
		CondominiumID: "condo-1",
		OwnerID:       "user-2",
	})

	if !resident.IsDuplicateError(err) {
		t.Errorf("expected duplicate error for same unit in same condominium, got %v", err)
	}
}

func TestResident_Create_SameUnitInDifferentCondominium_Succeeds(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, err := svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         1,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	_, err = svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         1,
		CondominiumID: "condo-2", // different condominium — must succeed
		OwnerID:       "user-2",
	})

	if err != nil {
		t.Errorf("same unit number in different condominium should succeed, got %v", err)
	}
}

func TestResident_Create_DifferentUnitInSameCondominium_Succeeds(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, _ = svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "101",
		Floor:         1,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	_, err := svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "102", // different unit — must succeed
		Floor:         1,
		CondominiumID: "condo-1",
		OwnerID:       "user-2",
	})

	if err != nil {
		t.Errorf("different unit in same condominium should succeed, got %v", err)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestResident_GetByID_ExistingResident_ReturnsResident(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, resident.CreateRequest{
		UnitNumber:    "201",
		Floor:         2,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	found, err := svc.GetByID(adminCtx(), created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestResident_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(adminCtx(), "ghost-id")
	if !resident.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestResident_List_ReturnsAllResidents(t *testing.T) {
	svc := newService()
	mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "u1"})
	mustCreate(t, svc, resident.CreateRequest{UnitNumber: "102", Floor: 1, CondominiumID: "condo-1", OwnerID: "u2"})

	list, err := svc.List(adminCtx(), page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list.Items) != 2 || list.Total != 2 {
		t.Errorf("expected 2 residents, got %d (total %d)", len(list.Items), list.Total)
	}
}

func TestResident_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	list, err := svc.List(adminCtx(), page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if list.Items == nil {
		t.Error("expected empty slice, not nil")
	}
	if list.Total != 0 || list.Page != 1 || list.PageSize != page.DefaultSize {
		t.Errorf("unexpected envelope: %+v", list)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestResident_Update_ChangesFloor(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, resident.CreateRequest{
		UnitNumber:    "301",
		Floor:         3,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	updated, err := svc.Update(adminCtx(), created.ID, resident.UpdateRequest{
		UnitNumber: "301",
		Floor:      5,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Floor != 5 {
		t.Errorf("expected floor 5, got %d", updated.Floor)
	}
}

func TestResident_Update_DuplicateUnitInSameCondominium_ReturnsDuplicateError(t *testing.T) {
	svc := newService()
	mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "u1"})
	r2 := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "102", Floor: 1, CondominiumID: "condo-1", OwnerID: "u2"})

	_, err := svc.Update(adminCtx(), r2.ID, resident.UpdateRequest{
		UnitNumber: "101", // already taken in condo-1
		Floor:      1,
	})

	if !resident.IsDuplicateError(err) {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestResident_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(adminCtx(), "ghost-id", resident.UpdateRequest{UnitNumber: "101", Floor: 1})
	if !resident.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestResident_Delete_ExistingResident_Succeeds(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, resident.CreateRequest{
		UnitNumber:    "401",
		Floor:         4,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	if err := svc.Delete(adminCtx(), created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(adminCtx(), created.ID)
	if !resident.IsNotFoundError(err) {
		t.Error("expected resident to be deleted")
	}
}

func TestResident_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(adminCtx(), "ghost-id")
	if !resident.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Security: authorization ───────────────────────────────────────────────────

func TestService_Security_UserCannotWrite_ReturnsForbidden(t *testing.T) {
	svc := newService()
	r := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "user-1"})
	owner := userCtx("user-1")

	if _, err := svc.Create(owner, resident.CreateRequest{UnitNumber: "102", Floor: 1, CondominiumID: "condo-1", OwnerID: "user-1"}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Create: expected ErrForbidden, got %v", err)
	}
	if _, err := svc.Update(owner, r.ID, resident.UpdateRequest{UnitNumber: "999", Floor: 9}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Update: expected ErrForbidden, got %v", err)
	}
	if err := svc.Delete(owner, r.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Delete: expected ErrForbidden, got %v", err)
	}

	got, err := svc.GetByID(adminCtx(), r.ID)
	if err != nil || got.UnitNumber != "101" || got.Floor != 1 {
		t.Errorf("unit must be unchanged, got %+v (%v)", got, err)
	}
	if list, _ := svc.List(adminCtx(), page.Default); list.Total != 1 {
		t.Errorf("forbidden create must not persist, total %d", list.Total)
	}
}

func TestService_Security_NoIdentity_ReturnsUnauthenticated(t *testing.T) {
	svc := newService()
	r := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "user-1"})
	anon := tenant.WithCondominiumID(context.Background(), condoA)

	if _, err := svc.Create(anon, resident.CreateRequest{UnitNumber: "102", CondominiumID: "c", OwnerID: "o"}); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("Create: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.GetByID(anon, r.ID); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("GetByID: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.List(anon, page.Default); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("List: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.Update(anon, r.ID, resident.UpdateRequest{Floor: 2}); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("Update: expected ErrUnauthenticated, got %v", err)
	}
	if err := svc.Delete(anon, r.ID); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("Delete: expected ErrUnauthenticated, got %v", err)
	}
}

func TestService_Security_UserSeesOnlyOwnUnits(t *testing.T) {
	svc := newService()
	a := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "user-a"})
	b := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "102", Floor: 1, CondominiumID: "condo-1", OwnerID: "user-b"})

	list, err := svc.List(userCtx("user-a"), page.Default)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != a.ID {
		t.Errorf("user-a must only list own unit, got %+v", list)
	}

	if _, err := svc.GetByID(userCtx("user-a"), a.ID); err != nil {
		t.Errorf("own unit: expected no error, got %v", err)
	}
	if _, err := svc.GetByID(userCtx("user-a"), b.ID); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("other user's unit: expected ErrNotFound, got %v", err)
	}
	if list, _ := svc.List(adminCtx(), page.Default); list.Total != 2 {
		t.Errorf("admin must see both units, got %d", list.Total)
	}
}

func TestService_Security_OtherCondominiumCannotAccessUnit(t *testing.T) {
	svc := newService()
	r := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "user-1"})
	adminB := identityCtx(condoB, "admin-1", authz.RoleAdmin)

	if _, err := svc.GetByID(adminB, r.ID); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("GetByID: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.Update(adminB, r.ID, resident.UpdateRequest{Floor: 9}); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("Update: expected ErrNotFound, got %v", err)
	}
	if err := svc.Delete(adminB, r.ID); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("Delete: expected ErrNotFound, got %v", err)
	}
	if list, _ := svc.List(adminB, page.Default); list.Total != 0 {
		t.Errorf("List: expected nothing, got %d", list.Total)
	}
	if got, err := svc.GetByID(adminCtx(), r.ID); err != nil || got.Floor != 1 {
		t.Errorf("unit must be unchanged in condo-a, got %+v (%v)", got, err)
	}
}

func TestService_Pagination_PageAndTotal(t *testing.T) {
	svc := newService()
	ids := make([]string, 0, 5)
	for _, u := range []string{"101", "102", "103", "104", "105"} {
		ids = append(ids, mustCreate(t, svc, resident.CreateRequest{UnitNumber: u, Floor: 1, CondominiumID: "condo-1", OwnerID: "o"}).ID)
	}

	res, err := svc.List(adminCtx(), page.Params{Page: 2, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Page != 2 || res.PageSize != 2 || res.Total != 5 || len(res.Items) != 2 {
		t.Fatalf("unexpected page: %+v", res)
	}
	if res.Items[0].ID != ids[2] || res.Items[1].ID != ids[3] {
		t.Errorf("expected items 3 and 4 in insertion order")
	}
	if res, _ := svc.List(adminCtx(), page.Params{Page: 9, Size: 2}); len(res.Items) != 0 || res.Total != 5 {
		t.Errorf("past the end: expected empty items, total 5, got %+v", res)
	}
}

func TestService_UnitIDsOwnedBy(t *testing.T) {
	svc := newService()
	a1 := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "101", CondominiumID: "c", OwnerID: "user-a"})
	mustCreate(t, svc, resident.CreateRequest{UnitNumber: "102", CondominiumID: "c", OwnerID: "user-b"})
	a2 := mustCreate(t, svc, resident.CreateRequest{UnitNumber: "103", CondominiumID: "c", OwnerID: "user-a"})

	ids, err := svc.UnitIDsOwnedBy(adminCtx(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)
	want := []string{a1.ID, a2.ID}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("expected %v, got %v", want, ids)
	}
	if ids, _ := svc.UnitIDsOwnedBy(identityCtx(condoB, "x", authz.RoleAdmin), "user-a"); len(ids) != 0 {
		t.Errorf("another condominium must see no units, got %v", ids)
	}
}

// ── In-memory repository: tenant scoping ──────────────────────────────────────

func TestInMemoryRepository_TenantScoped(t *testing.T) {
	repo := resident.NewInMemoryRepository()
	ctxA := tenant.WithCondominiumID(context.Background(), condoA)
	ctxB := tenant.WithCondominiumID(context.Background(), condoB)

	if err := repo.Save(context.Background(), &resident.Resident{UnitNumber: "1", CondominiumID: "c", OwnerID: "o"}); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("Save without tenant: expected ErrMissing, got %v", err)
	}
	r := &resident.Resident{UnitNumber: "1", CondominiumID: "c", OwnerID: "o"}
	if err := repo.Save(ctxA, r); err != nil {
		t.Fatal(err)
	}
	// The same unit in another tenant is not a duplicate.
	if err := repo.Save(ctxB, &resident.Resident{UnitNumber: "1", CondominiumID: "c", OwnerID: "o"}); err != nil {
		t.Errorf("Save in condo-b: %v", err)
	}
	if _, err := repo.FindByID(ctxB, r.ID); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("FindByID cross-tenant: expected ErrNotFound, got %v", err)
	}
	if err := repo.Update(ctxB, &resident.Resident{ID: r.ID, UnitNumber: "z"}); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("Update cross-tenant: expected ErrNotFound, got %v", err)
	}
	if err := repo.Delete(ctxB, r.ID); !errors.Is(err, resident.ErrNotFound) {
		t.Errorf("Delete cross-tenant: expected ErrNotFound, got %v", err)
	}
	if list, total, _ := repo.FindAll(ctxA, "", page.Default); total != 1 || len(list) != 1 || list[0].ID != r.ID {
		t.Errorf("FindAll condo-a: expected only r, got %v (total %d)", list, total)
	}
	if _, _, err := repo.FindAll(context.Background(), "", page.Default); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("FindAll without tenant: expected ErrMissing, got %v", err)
	}
	if _, err := repo.FindByID(context.Background(), r.ID); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("FindByID without tenant: expected ErrMissing, got %v", err)
	}
}
