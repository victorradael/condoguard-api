package expense_test

import (
	"context"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/expense"
)

func newService() *expense.Service {
	return expense.NewService(expense.NewInMemoryRepository())
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

	e, err := svc.Create(context.Background(), expense.CreateRequest{
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

	e, err := svc.Create(context.Background(), expense.CreateRequest{
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

func TestExpense_Create_ZeroAmount_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), expense.CreateRequest{
		Description: "Taxa",
		AmountCents: 0,
		DueDate:     mustDate("2026-03-31T00:00:00Z"),
		ResidentID:  "resident-1",
	})

	if !expense.IsValidationError(err) {
		t.Errorf("expected validation error for zero amount, got %v", err)
	}
}

func TestExpense_Create_NegativeAmount_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), expense.CreateRequest{
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

	_, err := svc.Create(context.Background(), expense.CreateRequest{
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

	_, err := svc.Create(context.Background(), expense.CreateRequest{
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

	_, err := svc.Create(context.Background(), expense.CreateRequest{
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
	ctx := context.Background()

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

	_, err := svc.GetByID(context.Background(), "ghost-id")
	if !expense.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestExpense_List_ReturnsAll(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "A", AmountCents: 100, DueDate: mustDate("2026-01-01T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "B", AmountCents: 200, DueDate: mustDate("2026-02-01T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "C", AmountCents: 300, DueDate: mustDate("2026-03-01T00:00:00Z"), ResidentID: "r1"})

	list, err := svc.List(ctx, expense.Filter{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 expenses, got %d", len(list))
	}
}

func TestExpense_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	list, err := svc.List(context.Background(), expense.Filter{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if list == nil {
		t.Error("expected empty slice, not nil")
	}
}

// ── Filtro por período ────────────────────────────────────────────────────────

func TestExpense_List_FilterByDateRange_ReturnsOnlyMatchingExpenses(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Jan", AmountCents: 100, DueDate: mustDate("2026-01-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Feb", AmountCents: 200, DueDate: mustDate("2026-02-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Mar", AmountCents: 300, DueDate: mustDate("2026-03-15T00:00:00Z"), ResidentID: "r1"})

	from := mustDate("2026-02-01T00:00:00Z")
	to := mustDate("2026-02-28T23:59:59Z")

	list, err := svc.List(ctx, expense.Filter{From: &from, To: &to})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 expense in Feb, got %d", len(list))
	}
	if list[0].Description != "Feb" {
		t.Errorf("expected 'Feb', got %q", list[0].Description)
	}
}

func TestExpense_List_FilterByFromOnly_ReturnsExpensesAfterFrom(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Jan", AmountCents: 100, DueDate: mustDate("2026-01-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Mar", AmountCents: 300, DueDate: mustDate("2026-03-15T00:00:00Z"), ResidentID: "r1"})

	from := mustDate("2026-02-01T00:00:00Z")
	list, err := svc.List(ctx, expense.Filter{From: &from})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 expense after Feb 2026, got %d", len(list))
	}
}

func TestExpense_List_FilterByToOnly_ReturnsExpensesBeforeTo(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Jan", AmountCents: 100, DueDate: mustDate("2026-01-15T00:00:00Z"), ResidentID: "r1"})
	_, _ = svc.Create(ctx, expense.CreateRequest{Description: "Mar", AmountCents: 300, DueDate: mustDate("2026-03-15T00:00:00Z"), ResidentID: "r1"})

	to := mustDate("2026-01-31T23:59:59Z")
	list, err := svc.List(ctx, expense.Filter{To: &to})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 expense before Feb 2026, got %d", len(list))
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestExpense_Update_ChangesDescriptionAndAmount(t *testing.T) {
	svc := newService()
	ctx := context.Background()

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
	ctx := context.Background()

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

	_, err := svc.Update(context.Background(), "ghost-id", expense.UpdateRequest{
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
	ctx := context.Background()

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

	err := svc.Delete(context.Background(), "ghost-id")
	if !expense.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}
