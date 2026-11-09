package expense_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Fixed ownership used by the tests: user-a owns resident res-a and shop
// shop-a, user-b owns resident res-b, anyone else owns nothing.
var testUnits = authz.UnitResolverFunc(func(_ context.Context, userID string) (authz.Units, error) {
	switch userID {
	case "user-a":
		return authz.Units{ResidentIDs: []string{"res-a"}, ShopOwnerIDs: []string{"shop-a"}}, nil
	case "user-b":
		return authz.Units{ResidentIDs: []string{"res-b"}}, nil
	}
	return authz.Units{}, nil
})

func newService() *expense.Service {
	return expense.NewService(expense.NewInMemoryRepository(), testUnits)
}

// callerCtx is the context of an authenticated caller of condoID.
func callerCtx(condoID, userID string, roles ...string) context.Context {
	return middleware.WithIdentity(tenant.WithCondominiumID(context.Background(), condoID), userID, roles)
}

func adminCtx() context.Context { return callerCtx("condo-a", "admin-1", authz.RoleAdmin) }

func userCtx(userID string) context.Context { return callerCtx("condo-a", userID, authz.RoleUser) }

func listAll(t *testing.T, svc *expense.Service, ctx context.Context, f expense.Filter) []*expense.Expense {
	t.Helper()
	res, err := svc.List(ctx, f, page.Default)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return res.Items
}

func mustDate(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestExpense_Create_Success_LinkedToResident(t *testing.T) {
	svc := newService()

	e, err := svc.Create(adminCtx(), expense.CreateRequest{
		Description: "Condomínio Março",
		AmountCents: 15000,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if e.ID == "" {
		t.Error("expected non-empty ID")
	}
	if e.AmountCents != 15000 {
		t.Errorf("expected AmountCents 15000, got %d", e.AmountCents)
	}
	if e.ResidentID != "resident-1" {
		t.Errorf("expected ResidentID 'resident-1', got %q", e.ResidentID)
	}
}

func TestExpense_Create_Success_LinkedToShopOwner(t *testing.T) {
	svc := newService()

	e, err := svc.Create(adminCtx(), expense.CreateRequest{
		Description: "Aluguel Loja",
		AmountCents: 300000,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ShopOwnerID: "shop-1",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if e.ShopOwnerID != "shop-1" {
		t.Errorf("expected ShopOwnerID 'shop-1', got %q", e.ShopOwnerID)
	}
}

// ── Validação: valor em centavos ──────────────────────────────────────────────

func TestExpense_Create_ZeroAmount_IsValid(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), expense.CreateRequest{
		Description: "Isenção",
		AmountCents: 0,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	if err != nil {
		t.Errorf("expected zero amount to be valid, got error: %v", err)
	}
}

func TestExpense_Create_NegativeAmount_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), expense.CreateRequest{
		Description: "Taxa",
		AmountCents: -100,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	if !expense.IsValidationError(err) {
		t.Errorf("expected validation error for negative amount, got %v", err)
	}
}

// ── Validação: data de vencimento ─────────────────────────────────────────────

func TestExpense_Create_MissingDueDate_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), expense.CreateRequest{
		Description: "Taxa",
		AmountCents: 1000,
		ResidentID:  "resident-1",
		// DueDate zero value
	})

	if !expense.IsValidationError(err) {
		t.Errorf("expected validation error for missing due date, got %v", err)
	}
}

// ── Validação: vínculo obrigatório ────────────────────────────────────────────

func TestExpense_Create_NoUnitLink_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), expense.CreateRequest{
		Description: "Taxa",
		AmountCents: 1000,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		// neither ResidentID nor ShopOwnerID
	})

	if !expense.IsValidationError(err) {
		t.Errorf("expected validation error when no unit is linked, got %v", err)
	}
}

func TestExpense_Create_MissingDescription_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), expense.CreateRequest{
		AmountCents: 1000,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	if !expense.IsValidationError(err) {
		t.Errorf("expected validation error for missing description, got %v", err)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestExpense_GetByID_Existing_ReturnsExpense(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created, _ := svc.Create(ctx, expense.CreateRequest{
		Description: "Energia",
		AmountCents: 8500,
		DueDate:     mustDate("2026-04-10T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	found, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestExpense_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(adminCtx(), "ghost-id")
	if !expense.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestExpense_List_ReturnsAll(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "A", AmountCents: 100, DueDate: mustDate("2026-01-01T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "B", AmountCents: 200, DueDate: mustDate("2026-02-01T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "C", AmountCents: 300, DueDate: mustDate("2026-03-01T00:00:00Z"), ResidentID: "r1"})

	list := listAll(t, svc, ctx, expense.Filter{})
	if len(list) != 3 {
		t.Errorf("expected 3 expenses, got %d", len(list))
	}
}

func TestExpense_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	res, err := svc.List(adminCtx(), expense.Filter{}, page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Items == nil || len(res.Items) != 0 || res.Total != 0 {
		t.Error("expected empty slice, not nil")
	}
}

// ── Filtro por período ────────────────────────────────────────────────────────

func TestExpense_List_FilterByDateRange_ReturnsOnlyMatchingExpenses(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Jan", AmountCents: 100, DueDate: mustDate("2026-01-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Feb", AmountCents: 200, DueDate: mustDate("2026-02-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Mar", AmountCents: 300, DueDate: mustDate("2026-03-15T00:00:00Z"), ResidentID: "r1"})

	from := mustDate("2026-02-01T00:00:00Z")
	to := mustDate("2026-02-28T23:59:59Z")

	list := listAll(t, svc, ctx, expense.Filter{From: &from, To: &to})
	if len(list) != 1 {
		t.Fatalf("expected 1 expense in Feb, got %d", len(list))
	}
	if list[0].Description != "Feb" {
		t.Errorf("expected 'Feb', got %q", list[0].Description)
	}
}

func TestExpense_List_FilterByFromOnly_ReturnsExpensesAfterFrom(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Jan", AmountCents: 100, DueDate: mustDate("2026-01-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Mar", AmountCents: 300, DueDate: mustDate("2026-03-15T00:00:00Z"), ResidentID: "r1"})

	from := mustDate("2026-02-01T00:00:00Z")
	list := listAll(t, svc, ctx, expense.Filter{From: &from})
	if len(list) != 1 {
		t.Errorf("expected 1 expense after Feb 2026, got %d", len(list))
	}
}

func TestExpense_List_FilterByToOnly_ReturnsExpensesBeforeTo(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Jan", AmountCents: 100, DueDate: mustDate("2026-01-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Mar", AmountCents: 300, DueDate: mustDate("2026-03-15T00:00:00Z"), ResidentID: "r1"})

	to := mustDate("2026-01-31T23:59:59Z")
	list := listAll(t, svc, ctx, expense.Filter{To: &to})
	if len(list) != 1 {
		t.Errorf("expected 1 expense before Feb 2026, got %d", len(list))
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestExpense_Update_ChangesDescriptionAndAmount(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created, _ := svc.Create(ctx, expense.CreateRequest{
		Description: "Água",
		AmountCents: 5000,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	updated, err := svc.Update(ctx, created.ID, expense.UpdateRequest{
		Description: "Água e Esgoto",
		AmountCents: 7500,
		DueDate:     mustDate("2026-04-15T00:00:00Z"),
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Description != "Água e Esgoto" {
		t.Errorf("expected 'Água e Esgoto', got %q", updated.Description)
	}
	if updated.AmountCents != 7500 {
		t.Errorf("expected AmountCents 7500, got %d", updated.AmountCents)
	}
}

func TestExpense_Update_NegativeAmount_ReturnsValidationError(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created, _ := svc.Create(ctx, expense.CreateRequest{
		Description: "Gás",
		AmountCents: 2000,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	_, err := svc.Update(ctx, created.ID, expense.UpdateRequest{
		AmountCents: -500,
	})

	if !expense.IsValidationError(err) {
		t.Errorf("expected validation error for negative amount, got %v", err)
	}
}

func TestExpense_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(adminCtx(), "ghost-id", expense.UpdateRequest{
		Description: "X",
		AmountCents: 100,
	})
	if !expense.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestExpense_Delete_Existing_Succeeds(t *testing.T) {
	svc := newService()
	ctx := adminCtx()

	created, _ := svc.Create(ctx, expense.CreateRequest{
		Description: "Internet",
		AmountCents: 9900,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if !expense.IsNotFoundError(err) {
		t.Error("expected expense to be deleted")
	}
}

func TestExpense_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(adminCtx(), "ghost-id")
	if !expense.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Security ──────────────────────────────────────────────────────────────────

func mustCreate(t *testing.T, svc *expense.Service, req expense.CreateRequest) *expense.Expense {
	t.Helper()
	if req.Description == "" {
		req.Description = "Taxa"
	}
	if req.DueDate.IsZero() {
		req.DueDate = mustDate("2026-03-31T00:00:00Z")
	}
	e, err := svc.Create(adminCtx(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return e
}

func TestExpense_Security_NoIdentity_ReturnsUnauthenticated(t *testing.T) {
	svc := newService()
	ctx := tenant.WithCondominiumID(context.Background(), "condo-a")
	e := mustCreate(t, svc, expense.CreateRequest{ResidentID: "res-a"})

	if _, err := svc.Create(ctx, expense.CreateRequest{Description: "x", DueDate: mustDate("2026-03-31T00:00:00Z"), ResidentID: "r"}); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("create: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.GetByID(ctx, e.ID); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("get: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.List(ctx, expense.Filter{}, page.Default); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("list: expected ErrUnauthenticated, got %v", err)
	}
}

func TestExpense_Security_UserCannotCreateUpdateDelete(t *testing.T) {
	svc := newService()
	e := mustCreate(t, svc, expense.CreateRequest{ResidentID: "res-a"})
	ctx := userCtx("user-a") // even the owner of the unit

	if _, err := svc.Create(ctx, expense.CreateRequest{Description: "x", AmountCents: 1, DueDate: mustDate("2026-03-31T00:00:00Z"), ResidentID: "res-a"}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("create: expected ErrForbidden, got %v", err)
	}
	if _, err := svc.Update(ctx, e.ID, expense.UpdateRequest{AmountCents: 1}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("update: expected ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, e.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("delete: expected ErrForbidden, got %v", err)
	}
	got, err := svc.GetByID(adminCtx(), e.ID)
	if err != nil || got.AmountCents != e.AmountCents {
		t.Errorf("expense must be untouched: %+v (%v)", got, err)
	}
}

func TestExpense_Security_UserSeesOnlyOwnUnits(t *testing.T) {
	svc := newService()
	resA := mustCreate(t, svc, expense.CreateRequest{Description: "res-a", DueDate: mustDate("2026-01-01T00:00:00Z"), ResidentID: "res-a"})
	shopA := mustCreate(t, svc, expense.CreateRequest{Description: "shop-a", DueDate: mustDate("2026-02-01T00:00:00Z"), ShopOwnerID: "shop-a"})
	resB := mustCreate(t, svc, expense.CreateRequest{Description: "res-b", DueDate: mustDate("2026-03-01T00:00:00Z"), ResidentID: "res-b"})

	list := listAll(t, svc, userCtx("user-a"), expense.Filter{})
	if len(list) != 2 || list[0].ID != resA.ID || list[1].ID != shopA.ID {
		t.Fatalf("user-a: expected [res-a shop-a], got %+v", list)
	}
	list = listAll(t, svc, userCtx("user-b"), expense.Filter{})
	if len(list) != 1 || list[0].ID != resB.ID {
		t.Fatalf("user-b: expected [res-b], got %+v", list)
	}
	res, err := svc.List(userCtx("nobody"), expense.Filter{}, page.Default)
	if err != nil || len(res.Items) != 0 || res.Total != 0 {
		t.Fatalf("user without units: expected empty list, got %+v (%v)", res, err)
	}
	if list := listAll(t, svc, adminCtx(), expense.Filter{}); len(list) != 3 {
		t.Errorf("admin: expected 3, got %d", len(list))
	}

	if _, err := svc.GetByID(userCtx("user-a"), shopA.ID); err != nil {
		t.Errorf("user-a own shop expense: %v", err)
	}
	if _, err := svc.GetByID(userCtx("user-a"), resB.ID); !expense.IsNotFoundError(err) {
		t.Errorf("user-a reading user-b expense: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.GetByID(userCtx("nobody"), resA.ID); !expense.IsNotFoundError(err) {
		t.Errorf("user without units: expected ErrNotFound, got %v", err)
	}
}

func TestExpense_Security_ListIgnoresScopeFromCaller(t *testing.T) {
	svc := newService()
	mustCreate(t, svc, expense.CreateRequest{ResidentID: "res-a"})
	mustCreate(t, svc, expense.CreateRequest{ResidentID: "res-b"})

	list := listAll(t, svc, userCtx("user-a"), expense.Filter{Scope: authz.Scope{All: true}})
	if len(list) != 1 || list[0].ResidentID != "res-a" {
		t.Fatalf("forged Scope.All must be ignored, got %+v", list)
	}
	list = listAll(t, svc, userCtx("user-a"), expense.Filter{Scope: authz.Scope{Units: authz.Units{ResidentIDs: []string{"res-b"}}}})
	if len(list) != 1 || list[0].ResidentID != "res-a" {
		t.Fatalf("forged Scope.Units must be ignored, got %+v", list)
	}
}

func TestExpense_Security_OtherCondominiumAdminCannotAccess(t *testing.T) {
	svc := newService()
	e := mustCreate(t, svc, expense.CreateRequest{ResidentID: "res-a"})
	other := callerCtx("condo-b", "admin-b", authz.RoleAdmin)

	if _, err := svc.GetByID(other, e.ID); !expense.IsNotFoundError(err) {
		t.Errorf("get: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.Update(other, e.ID, expense.UpdateRequest{AmountCents: 1}); !expense.IsNotFoundError(err) {
		t.Errorf("update: expected ErrNotFound, got %v", err)
	}
	if err := svc.Delete(other, e.ID); !expense.IsNotFoundError(err) {
		t.Errorf("delete: expected ErrNotFound, got %v", err)
	}
	if list := listAll(t, svc, other, expense.Filter{}); len(list) != 0 {
		t.Errorf("list: expected nothing from condo-a, got %+v", list)
	}
	if got, err := svc.GetByID(adminCtx(), e.ID); err != nil || got.AmountCents != e.AmountCents {
		t.Errorf("condo-a expense must be untouched: %+v (%v)", got, err)
	}
}

func TestExpense_List_Pagination(t *testing.T) {
	svc := newService()
	for _, d := range []string{"2026-01-05", "2026-01-01", "2026-01-04", "2026-01-02", "2026-01-03"} {
		mustCreate(t, svc, expense.CreateRequest{Description: d, DueDate: mustDate(d + "T00:00:00Z"), ResidentID: "res-a"})
	}

	res, err := svc.List(adminCtx(), expense.Filter{}, page.Params{Page: 2, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 5 || res.Page != 2 || res.PageSize != 2 || len(res.Items) != 2 ||
		res.Items[0].Description != "2026-01-03" || res.Items[1].Description != "2026-01-04" {
		t.Fatalf("unexpected page: %+v", res)
	}
	res, _ = svc.List(adminCtx(), expense.Filter{}, page.Params{Page: 3, Size: 2})
	if len(res.Items) != 1 || res.Items[0].Description != "2026-01-05" {
		t.Errorf("last page: %+v", res.Items)
	}
	res, _ = svc.List(adminCtx(), expense.Filter{}, page.Params{Page: 9, Size: 2})
	if res.Items == nil || len(res.Items) != 0 || res.Total != 5 {
		t.Errorf("past the end: expected empty items and total 5, got %+v", res)
	}
}

func TestInMemoryRepository_TenantScoped(t *testing.T) {
	repo := expense.NewInMemoryRepository()
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")
	e := &expense.Expense{Description: "x", DueDate: mustDate("2026-03-31T00:00:00Z"), ResidentID: "r"}
	if err := repo.Save(ctxA, e); err != nil {
		t.Fatal(err)
	}

	if err := repo.Save(context.Background(), &expense.Expense{}); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("save without tenant: expected ErrMissing, got %v", err)
	}
	if _, err := repo.FindByID(context.Background(), e.ID); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("find without tenant: expected ErrMissing, got %v", err)
	}
	if _, _, err := repo.FindAll(context.Background(), expense.Filter{Scope: authz.Scope{All: true}}, page.Default); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("list without tenant: expected ErrMissing, got %v", err)
	}
	if _, err := repo.FindByID(ctxB, e.ID); !errors.Is(err, expense.ErrNotFound) {
		t.Errorf("other tenant find: expected ErrNotFound, got %v", err)
	}
	if err := repo.Update(ctxB, e); !errors.Is(err, expense.ErrNotFound) {
		t.Errorf("other tenant update: expected ErrNotFound, got %v", err)
	}
	if err := repo.Delete(ctxB, e.ID); !errors.Is(err, expense.ErrNotFound) {
		t.Errorf("other tenant delete: expected ErrNotFound, got %v", err)
	}
	if list, total, _ := repo.FindAll(ctxB, expense.Filter{Scope: authz.Scope{All: true}}, page.Default); len(list) != 0 || total != 0 {
		t.Errorf("other tenant list: expected nothing, got %d/%d", len(list), total)
	}
	if _, err := repo.FindByID(ctxA, e.ID); err != nil {
		t.Errorf("own tenant: %v", err)
	}
}
