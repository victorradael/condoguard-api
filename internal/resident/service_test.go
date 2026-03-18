package resident_test

import (
	"context"
	"testing"

	"github.com/victorradael/condoguard/api/internal/resident"
)

func newService() *resident.Service {
	return resident.NewService(resident.NewInMemoryRepository())
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestResident_Create_Success(t *testing.T) {
	svc := newService()

	r, err := svc.Create(context.Background(), resident.CreateRequest{
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

	_, err := svc.Create(context.Background(), resident.CreateRequest{
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

	_, err := svc.Create(context.Background(), resident.CreateRequest{
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

	_, err := svc.Create(context.Background(), resident.CreateRequest{
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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

	created, _ := svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "201",
		Floor:         2,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	found, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestResident_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(context.Background(), "ghost-id")
	if !resident.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestResident_List_ReturnsAllResidents(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "u1"})
	_, _ = svc.Create(ctx, resident.CreateRequest{UnitNumber: "102", Floor: 1, CondominiumID: "condo-1", OwnerID: "u2"})

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 residents, got %d", len(list))
	}
}

func TestResident_List_Empty_ReturnsEmptySlice(t *testing.T) {
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

func TestResident_Update_ChangesFloor(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "301",
		Floor:         3,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	updated, err := svc.Update(ctx, created.ID, resident.UpdateRequest{
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
	ctx := context.Background()

	_, _ = svc.Create(ctx, resident.CreateRequest{UnitNumber: "101", Floor: 1, CondominiumID: "condo-1", OwnerID: "u1"})
	r2, _ := svc.Create(ctx, resident.CreateRequest{UnitNumber: "102", Floor: 1, CondominiumID: "condo-1", OwnerID: "u2"})

	_, err := svc.Update(ctx, r2.ID, resident.UpdateRequest{
		UnitNumber: "101", // already taken in condo-1
		Floor:      1,
	})

	if !resident.IsDuplicateError(err) {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestResident_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(context.Background(), "ghost-id", resident.UpdateRequest{UnitNumber: "101", Floor: 1})
	if !resident.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestResident_Delete_ExistingResident_Succeeds(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, resident.CreateRequest{
		UnitNumber:    "401",
		Floor:         4,
		CondominiumID: "condo-1",
		OwnerID:       "user-1",
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if !resident.IsNotFoundError(err) {
		t.Error("expected resident to be deleted")
	}
}

func TestResident_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(context.Background(), "ghost-id")
	if !resident.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}
