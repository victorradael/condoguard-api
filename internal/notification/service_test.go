package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/notification"
)

func newService() *notification.Service {
	return notification.NewService(notification.NewInMemoryRepository())
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestNotification_Create_Success(t *testing.T) {
	svc := newService()

	n, err := svc.Create(context.Background(), notification.CreateRequest{
		Message:     "Reunião de condomínio amanhã às 19h",
		CreatedByID: "user-1",
		ResidentIDs: []string{"resident-1", "resident-2"},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n.ID == "" {
		t.Error("expected non-empty ID")
	}
	if n.Read {
		t.Error("new notification must be unread")
	}
	if n.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestNotification_Create_WithShopOwners(t *testing.T) {
	svc := newService()

	n, err := svc.Create(context.Background(), notification.CreateRequest{
		Message:      "Manutenção no elevador",
		CreatedByID:  "user-1",
		ShopOwnerIDs: []string{"shop-1"},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(n.ShopOwnerIDs) != 1 || n.ShopOwnerIDs[0] != "shop-1" {
		t.Errorf("expected ShopOwnerIDs [shop-1], got %v", n.ShopOwnerIDs)
	}
}

func TestNotification_Create_WithResidentsAndShopOwners(t *testing.T) {
	svc := newService()

	n, err := svc.Create(context.Background(), notification.CreateRequest{
		Message:      "Aviso geral",
		CreatedByID:  "user-1",
		ResidentIDs:  []string{"r1"},
		ShopOwnerIDs: []string{"s1"},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(n.ResidentIDs) != 1 || len(n.ShopOwnerIDs) != 1 {
		t.Errorf("expected 1 resident and 1 shopowner, got %v / %v", n.ResidentIDs, n.ShopOwnerIDs)
	}
}

func TestNotification_Create_MissingMessage_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), notification.CreateRequest{
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	if !notification.IsValidationError(err) {
		t.Errorf("expected validation error for missing message, got %v", err)
	}
}

func TestNotification_Create_MissingCreatedBy_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), notification.CreateRequest{
		Message:     "Aviso",
		ResidentIDs: []string{"r1"},
	})

	if !notification.IsValidationError(err) {
		t.Errorf("expected validation error for missing createdBy, got %v", err)
	}
}

func TestNotification_Create_NoRecipients_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), notification.CreateRequest{
		Message:     "Aviso sem destinatário",
		CreatedByID: "user-1",
	})

	if !notification.IsValidationError(err) {
		t.Errorf("expected validation error for no recipients, got %v", err)
	}
}

// ── MarkAsRead — transição não lida → lida ────────────────────────────────────

func TestNotification_MarkAsRead_UnreadBecomesRead(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	n, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	if n.Read {
		t.Fatal("notification must start as unread")
	}

	updated, err := svc.MarkAsRead(ctx, n.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !updated.Read {
		t.Error("expected notification to be marked as read")
	}
}

func TestNotification_MarkAsRead_AlreadyRead_IsIdempotent(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	n, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	_, _ = svc.MarkAsRead(ctx, n.ID)
	// call again — must not error and must still be read
	updated, err := svc.MarkAsRead(ctx, n.ID)
	if err != nil {
		t.Fatalf("MarkAsRead must be idempotent, got error: %v", err)
	}
	if !updated.Read {
		t.Error("expected notification to remain read")
	}
}

func TestNotification_MarkAsRead_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.MarkAsRead(context.Background(), "ghost-id")
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── ReadAt is set on first MarkAsRead ─────────────────────────────────────────

func TestNotification_MarkAsRead_SetsReadAt(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	n, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	before := time.Now()
	updated, _ := svc.MarkAsRead(ctx, n.ID)
	after := time.Now()

	if updated.ReadAt == nil {
		t.Fatal("expected ReadAt to be set after MarkAsRead")
	}
	if updated.ReadAt.Before(before) || updated.ReadAt.After(after) {
		t.Errorf("ReadAt %v outside expected window [%v, %v]", updated.ReadAt, before, after)
	}
}

func TestNotification_MarkAsRead_IdempotentDoesNotUpdateReadAt(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	n, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	first, _ := svc.MarkAsRead(ctx, n.ID)
	firstReadAt := *first.ReadAt

	time.Sleep(2 * time.Millisecond)
	second, _ := svc.MarkAsRead(ctx, n.ID)

	if !second.ReadAt.Equal(firstReadAt) {
		t.Errorf("ReadAt must not change on idempotent call: first=%v second=%v", firstReadAt, second.ReadAt)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestNotification_GetByID_Existing_ReturnsNotification(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	found, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestNotification_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(context.Background(), "ghost-id")
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestNotification_List_ReturnsAll(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, notification.CreateRequest{Message: "A", CreatedByID: "u1", ResidentIDs: []string{"r1"}})
	_, _ = svc.Create(ctx, notification.CreateRequest{Message: "B", CreatedByID: "u1", ResidentIDs: []string{"r1"}})

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(list))
	}
}

func TestNotification_List_Empty_ReturnsEmptySlice(t *testing.T) {
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

func TestNotification_Update_ChangesMessage(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Mensagem original",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	updated, err := svc.Update(ctx, created.ID, notification.UpdateRequest{
		Message: "Mensagem atualizada",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Message != "Mensagem atualizada" {
		t.Errorf("expected 'Mensagem atualizada', got %q", updated.Message)
	}
}

func TestNotification_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(context.Background(), "ghost-id", notification.UpdateRequest{Message: "X"})
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestNotification_Delete_Existing_Succeeds(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		CreatedByID: "user-1",
		ResidentIDs: []string{"r1"},
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if !notification.IsNotFoundError(err) {
		t.Error("expected notification to be deleted")
	}
}

func TestNotification_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(context.Background(), "ghost-id")
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}
