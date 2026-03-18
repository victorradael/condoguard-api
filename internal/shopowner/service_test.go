package shopowner_test

import (
	"context"
	"testing"

	"github.com/victorradael/condoguard/api/internal/shopowner"
)

func newService() *shopowner.Service {
	return shopowner.NewService(shopowner.NewInMemoryRepository())
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestShopOwner_Create_Success(t *testing.T) {
	svc := newService()

	s, err := svc.Create(context.Background(), shopowner.CreateRequest{
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

	s, err := svc.Create(context.Background(), shopowner.CreateRequest{
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

	_, err := svc.Create(context.Background(), shopowner.CreateRequest{
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

	_, err := svc.Create(context.Background(), shopowner.CreateRequest{
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

	_, err := svc.Create(context.Background(), shopowner.CreateRequest{
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
	ctx := context.Background()

	_, err := svc.Create(ctx, shopowner.CreateRequest{
		ShopName: "Loja A",
		CNPJ:     "11.222.333/0001-81",
		Floor:    1,
		OwnerID:  "user-1",
	})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	_, err = svc.Create(ctx, shopowner.CreateRequest{
		ShopName: "Loja B",
		CNPJ:     "11222333000181", // same CNPJ, raw format
		Floor:    2,
		OwnerID:  "user-2",
	})

	if !shopowner.IsDuplicateError(err) {
		t.Errorf("expected duplicate error for same CNPJ, got %v", err)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestShopOwner_GetByID_ExistingShop_ReturnsShop(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, shopowner.CreateRequest{
		ShopName: "Barbearia Top",
		CNPJ:     "45.997.418/0001-53",
		Floor:    2,
		OwnerID:  "user-1",
	})

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

	_, err := svc.GetByID(context.Background(), "ghost-id")
	if !shopowner.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestShopOwner_List_ReturnsAll(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, shopowner.CreateRequest{ShopName: "A", CNPJ: "11.222.333/0001-81", Floor: 1, OwnerID: "u1"})
	_, _ = svc.Create(ctx, shopowner.CreateRequest{ShopName: "B", CNPJ: "45.997.418/0001-53", Floor: 2, OwnerID: "u2"})

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 shopowners, got %d", len(list))
	}
}

func TestShopOwner_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if list == nil {
		t.Error("expected empty slice, not nil")
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestShopOwner_Update_ChangesShopNameAndFloor(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, shopowner.CreateRequest{
		ShopName: "Mercado Velho",
		CNPJ:     "11.222.333/0001-81",
		Floor:    1,
		OwnerID:  "user-1",
	})

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

	_, err := svc.Update(context.Background(), "ghost-id", shopowner.UpdateRequest{ShopName: "X"})
	if !shopowner.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestShopOwner_Delete_ExistingShop_Succeeds(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, shopowner.CreateRequest{
		ShopName: "Farmácia",
		CNPJ:     "11.222.333/0001-81",
		Floor:    1,
		OwnerID:  "user-1",
	})

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

	err := svc.Delete(context.Background(), "ghost-id")
	if !shopowner.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}
