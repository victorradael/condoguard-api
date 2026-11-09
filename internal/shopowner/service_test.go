package shopowner_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/shopowner"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

func newService() *shopowner.Service {
	return shopowner.NewService(shopowner.NewInMemoryRepository())
}

func ctxAs(condo, userID string, roles ...string) context.Context {
	ctx := tenant.WithCondominiumID(context.Background(), condo)
	return middleware.WithIdentity(ctx, userID, roles)
}

// adminCtx is an admin of condo-a.
func adminCtx() context.Context { return ctxAs(condoA, "admin-1", authz.RoleAdmin) }

// userCtx is a ROLE_USER of condo-a.
func userCtx(userID string) context.Context { return ctxAs(condoA, userID, authz.RoleUser) }

func mustCreate(t *testing.T, svc *shopowner.Service, ctx context.Context, name, cnpj, owner string) *shopowner.ShopOwner {
	t.Helper()
	s, err := svc.Create(ctx, shopowner.CreateRequest{ShopName: name, CNPJ: cnpj, Floor: 1, OwnerID: owner})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return s
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestShopOwner_Create_Success(t *testing.T) {
	svc := newService()

	s, err := svc.Create(adminCtx(), shopowner.CreateRequest{
		ShopName: "Padaria Central",
		CNPJ:     "11.222.333/0001-81",
		Floor:    1,
		OwnerID:  "user-1",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.ShopName != "Padaria Central" {
		t.Errorf("expected ShopName 'Padaria Central', got %q", s.ShopName)
	}
}

func TestShopOwner_Create_CNPJIsFormattedOnSave(t *testing.T) {
	svc := newService()

	s, err := svc.Create(adminCtx(), shopowner.CreateRequest{
		ShopName: "Loja X",
		CNPJ:     "11222333000181", // raw digits, no formatting
		Floor:    1,
		OwnerID:  "user-1",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.CNPJ != "11.222.333/0001-81" {
		t.Errorf("expected formatted CNPJ, got %q", s.CNPJ)
	}
}

func TestShopOwner_Create_InvalidCNPJ_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), shopowner.CreateRequest{
		ShopName: "Loja Y",
		CNPJ:     "00000000000000",
		Floor:    1,
		OwnerID:  "user-1",
	})

	if !shopowner.IsValidationError(err) {
		t.Errorf("expected validation error for invalid CNPJ, got %v", err)
	}
}

func TestShopOwner_Create_MissingShopName_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), shopowner.CreateRequest{
		CNPJ:    "11.222.333/0001-81",
		Floor:   1,
		OwnerID: "user-1",
	})

	if !shopowner.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestShopOwner_Create_MissingOwnerID_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), shopowner.CreateRequest{
		ShopName: "Loja Z",
		CNPJ:     "11.222.333/0001-81",
		Floor:    1,
	})

	if !shopowner.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestShopOwner_Create_DuplicateCNPJ_ReturnsDuplicateError(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	mustCreate(t, svc, ctx, "Loja A", "11.222.333/0001-81", "user-1")

	_, err := svc.Create(ctx, shopowner.CreateRequest{
		ShopName: "Loja B",
		CNPJ:     "11222333000181", // same CNPJ, raw format
		Floor:    2,
		OwnerID:  "user-2",
	})

	if !shopowner.IsDuplicateError(err) {
		t.Errorf("expected duplicate error for same CNPJ, got %v", err)
	}
}

func TestShopOwner_Create_SameCNPJInOtherCondominium_Succeeds(t *testing.T) {
	svc := newService()
	mustCreate(t, svc, adminCtx(), "Loja A", "11.222.333/0001-81", "user-1")
	mustCreate(t, svc, ctxAs(condoB, "admin-b", authz.RoleAdmin), "Loja B", "11222333000181", "user-1")
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestShopOwner_GetByID_ExistingShop_ReturnsShop(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created := mustCreate(t, svc, ctx, "Barbearia Top", "45.997.418/0001-53", "user-1")

	found, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestShopOwner_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(adminCtx(), "ghost-id")
	if !shopowner.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestShopOwner_List_ReturnsAll(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	mustCreate(t, svc, ctx, "A", "11.222.333/0001-81", "u1")
	mustCreate(t, svc, ctx, "B", "45.997.418/0001-53", "u2")

	res, err := svc.List(ctx, page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(res.Items) != 2 || res.Total != 2 {
		t.Errorf("expected 2 shopowners, got %d items / total %d", len(res.Items), res.Total)
	}
	if res.Items[0].ShopName != "A" || res.Items[1].ShopName != "B" {
		t.Errorf("expected insertion order A, B; got %q, %q", res.Items[0].ShopName, res.Items[1].ShopName)
	}
	if res.Page != 1 || res.PageSize != page.DefaultSize {
		t.Errorf("unexpected page envelope: page=%d pageSize=%d", res.Page, res.PageSize)
	}
}

func TestShopOwner_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	res, err := svc.List(adminCtx(), page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Items == nil {
		t.Error("expected empty slice, not nil")
	}
	if res.Total != 0 {
		t.Errorf("expected total 0, got %d", res.Total)
	}
}

func TestShopOwner_List_Paginates(t *testing.T) {
	svc := newService()
	ctx := adminCtx()
	mustCreate(t, svc, ctx, "A", "11.222.333/0001-81", "u1")
	mustCreate(t, svc, ctx, "B", "45.997.418/0001-53", "u1")
	mustCreate(t, svc, ctx, "C", "11.444.777/0001-61", "u1")

	res, err := svc.List(ctx, page.Params{Page: 2, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || len(res.Items) != 1 || res.Items[0].ShopName != "C" || res.Page != 2 || res.PageSize != 2 {
		t.Errorf("page 2/2: %+v", res)
	}
	if res, _ := svc.List(ctx, page.Params{Page: 5, Size: 2}); res.Total != 3 || len(res.Items) != 0 || res.Items == nil {
		t.Errorf("page past the end: %+v", res)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestShopOwner_Update_ChangesShopNameAndFloor(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created := mustCreate(t, svc, ctx, "Mercado Velho", "11.222.333/0001-81", "user-1")

	updated, err := svc.Update(ctx, created.ID, shopowner.UpdateRequest{
		ShopName: "Mercado Novo",
		Floor:    3,
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.ShopName != "Mercado Novo" {
		t.Errorf("expected ShopName 'Mercado Novo', got %q", updated.ShopName)
	}
	if updated.Floor != 3 {
		t.Errorf("expected floor 3, got %d", updated.Floor)
	}
}

func TestShopOwner_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(adminCtx(), "ghost-id", shopowner.UpdateRequest{ShopName: "X"})
	if !shopowner.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestShopOwner_Delete_ExistingShop_Succeeds(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created := mustCreate(t, svc, ctx, "Farmácia", "11.222.333/0001-81", "user-1")

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if !shopowner.IsNotFoundError(err) {
		t.Error("expected shopowner to be deleted")
	}
}

func TestShopOwner_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(adminCtx(), "ghost-id")
	if !shopowner.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── UnitIDsOwnedBy ────────────────────────────────────────────────────────────

func TestShopOwner_UnitIDsOwnedBy_ReturnsOnlyOwnersShopsInTenant(t *testing.T) {
	svc := newService()
	a1 := mustCreate(t, svc, adminCtx(), "A1", "11.222.333/0001-81", "user-a")
	mustCreate(t, svc, adminCtx(), "B1", "45.997.418/0001-53", "user-b")
	a2 := mustCreate(t, svc, adminCtx(), "A2", "11.444.777/0001-61", "user-a")
	mustCreate(t, svc, ctxAs(condoB, "admin-b", authz.RoleAdmin), "A in B", "11.222.333/0001-81", "user-a")

	ids, err := svc.UnitIDsOwnedBy(userCtx("user-a"), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(ids)
	want := []string{a1.ID, a2.ID}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("expected %v, got %v", want, ids)
	}
	if ids, _ := svc.UnitIDsOwnedBy(userCtx("nobody"), "nobody"); len(ids) != 0 {
		t.Errorf("user without shops: expected none, got %v", ids)
	}
}

// ── Security ──────────────────────────────────────────────────────────────────

func TestService_Security_NoIdentity_ReturnsUnauthenticated(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, adminCtx(), "A", "11.222.333/0001-81", "user-1")
	ctx := tenant.WithCondominiumID(context.Background(), condoA) // tenant, but nobody logged in

	if _, err := svc.Create(ctx, shopowner.CreateRequest{ShopName: "X", CNPJ: "45997418000153", OwnerID: "u"}); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("Create: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.GetByID(ctx, created.ID); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("GetByID: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.List(ctx, page.Default); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("List: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.Update(ctx, created.ID, shopowner.UpdateRequest{ShopName: "X"}); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("Update: expected ErrUnauthenticated, got %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("Delete: expected ErrUnauthenticated, got %v", err)
	}
}

func TestService_Security_MissingTenant_FailsClosed(t *testing.T) {
	svc := newService()
	ctx := middleware.WithIdentity(context.Background(), "admin-1", []string{authz.RoleAdmin})

	if _, err := svc.Create(ctx, shopowner.CreateRequest{ShopName: "X", CNPJ: "11222333000181", OwnerID: "u"}); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("Create: expected tenant.ErrMissing, got %v", err)
	}
	if _, err := svc.List(ctx, page.Default); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("List: expected tenant.ErrMissing, got %v", err)
	}
}

func TestService_Security_UserCannotWrite_ReturnsForbidden(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, adminCtx(), "Original", "11.222.333/0001-81", "user-1")
	ctx := userCtx("user-1") // the owner themself

	if _, err := svc.Create(ctx, shopowner.CreateRequest{ShopName: "X", CNPJ: "45997418000153", OwnerID: "user-1"}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Create: expected ErrForbidden, got %v", err)
	}
	if _, err := svc.Update(ctx, created.ID, shopowner.UpdateRequest{ShopName: "Hacked"}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Update: expected ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Delete: expected ErrForbidden, got %v", err)
	}

	res, _ := svc.List(adminCtx(), page.Default)
	if res.Total != 1 || res.Items[0].ShopName != "Original" {
		t.Errorf("forbidden calls changed data: %+v", res.Items)
	}
}

func TestService_Security_UserSeesOnlyOwnShops(t *testing.T) {
	svc := newService()
	own := mustCreate(t, svc, adminCtx(), "A", "11.222.333/0001-81", "user-a")
	other := mustCreate(t, svc, adminCtx(), "B", "45.997.418/0001-53", "user-b")

	if got, err := svc.GetByID(userCtx("user-a"), own.ID); err != nil || got.ID != own.ID {
		t.Errorf("own shop: %v, %v", got, err)
	}
	if _, err := svc.GetByID(userCtx("user-a"), other.ID); !shopowner.IsNotFoundError(err) {
		t.Errorf("another user's shop: expected ErrNotFound, got %v", err)
	}

	res, err := svc.List(userCtx("user-a"), page.Default)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0].ID != own.ID {
		t.Errorf("user-a list: %+v", res)
	}
	if res, _ := svc.List(adminCtx(), page.Default); res.Total != 2 {
		t.Errorf("admin list: expected 2, got %d", res.Total)
	}
}

func TestService_Security_OtherCondominium_SeesNothing(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, adminCtx(), "A", "11.222.333/0001-81", "user-1")
	adminB := ctxAs(condoB, "admin-b", authz.RoleAdmin)

	if _, err := svc.GetByID(adminB, created.ID); !shopowner.IsNotFoundError(err) {
		t.Errorf("GetByID: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.Update(adminB, created.ID, shopowner.UpdateRequest{ShopName: "Hacked"}); !shopowner.IsNotFoundError(err) {
		t.Errorf("Update: expected ErrNotFound, got %v", err)
	}
	if err := svc.Delete(adminB, created.ID); !shopowner.IsNotFoundError(err) {
		t.Errorf("Delete: expected ErrNotFound, got %v", err)
	}
	if res, _ := svc.List(adminB, page.Default); res.Total != 0 || len(res.Items) != 0 {
		t.Errorf("List: expected empty, got %+v", res)
	}
	if got, err := svc.GetByID(adminCtx(), created.ID); err != nil || got.ShopName != "A" {
		t.Errorf("shop of condo-a changed: %v, %v", got, err)
	}
}
