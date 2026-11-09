package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/notification"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/page"
)

const testCondo = "condo-a"

// testUnits resolves the units owned by each test user:
//   - res-1 and res-2 both live in unit r1 (two recipients of the same notification)
//   - shop-1 owns shop s1
//   - outsider lives in unit r9 (never addressed in these tests)
var testUnits = authz.UnitResolverFunc(func(_ context.Context, userID string) (authz.Units, error) {
	switch userID {
	case "res-1", "res-2":
		return authz.Units{ResidentIDs: []string{"r1"}}, nil
	case "shop-1":
		return authz.Units{ShopOwnerIDs: []string{"s1"}}, nil
	case "outsider":
		return authz.Units{ResidentIDs: []string{"r9"}}, nil
	}
	return authz.Units{}, nil
})

func newService() *notification.Service {
	return notification.NewService(notification.NewInMemoryRepository(), testUnits)
}

func asUser(userID string, roles ...string) context.Context {
	ctx := tenant.WithCondominiumID(context.Background(), testCondo)
	return middleware.WithIdentity(ctx, userID, roles)
}

func adminCtx() context.Context         { return asUser("admin-1", authz.RoleAdmin) }
func userCtx(id string) context.Context { return asUser(id, authz.RoleUser) }

func mustCreate(t *testing.T, svc *notification.Service, req notification.CreateRequest) *notification.Notification {
	t.Helper()
	n, err := svc.Create(adminCtx(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return n
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestNotification_Create_Success(t *testing.T) {
	svc := newService()

	n, err := svc.Create(adminCtx(), notification.CreateRequest{
		Message:     "Reunião de condomínio amanhã às 19h",
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
	if n.CreatedByID != "admin-1" {
		t.Errorf("createdById must be the caller, got %q", n.CreatedByID)
	}
}

func TestNotification_Create_WithShopOwners(t *testing.T) {
	svc := newService()

	n, err := svc.Create(adminCtx(), notification.CreateRequest{
		Message:      "Manutenção no elevador",
		ShopOwnerIDs: []string{"shop-1"},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(n.ShopOwnerIDs) != 1 || n.ShopOwnerIDs[0] != "shop-1" {
		t.Errorf("expected ShopOwnerIDs [shop-1], got %v", n.ShopOwnerIDs)
	}
	if n.ResidentIDs == nil {
		t.Error("ResidentIDs must be an empty slice, not nil")
	}
}

func TestNotification_Create_WithResidentsAndShopOwners(t *testing.T) {
	svc := newService()

	n, err := svc.Create(adminCtx(), notification.CreateRequest{
		Message:      "Aviso geral",
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

	_, err := svc.Create(adminCtx(), notification.CreateRequest{
		ResidentIDs: []string{"r1"},
	})

	if !notification.IsValidationError(err) {
		t.Errorf("expected validation error for missing message, got %v", err)
	}
}

// The author used to come from the request; now it comes from the caller, so
// a call without an authenticated caller is rejected instead.
func TestNotification_Create_NoCaller_ReturnsUnauthenticated(t *testing.T) {
	svc := newService()
	ctx := tenant.WithCondominiumID(context.Background(), testCondo)

	_, err := svc.Create(ctx, notification.CreateRequest{
		Message:     "Aviso",
		ResidentIDs: []string{"r1"},
	})

	if !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestNotification_Create_NoRecipients_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(adminCtx(), notification.CreateRequest{
		Message: "Aviso sem destinatário",
	})

	if !notification.IsValidationError(err) {
		t.Errorf("expected validation error for no recipients, got %v", err)
	}
}

// ── MarkAsRead — transição não lida → lida ────────────────────────────────────

func TestNotification_MarkAsRead_UnreadBecomesRead(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})

	if n.Read {
		t.Fatal("notification must start as unread")
	}

	updated, err := svc.MarkAsRead(userCtx("res-1"), n.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !updated.Read {
		t.Error("expected notification to be marked as read")
	}
}

func TestNotification_MarkAsRead_AlreadyRead_IsIdempotent(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})
	ctx := userCtx("res-1")

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

	_, err := svc.MarkAsRead(adminCtx(), "ghost-id")
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── ReadAt is set on first MarkAsRead ─────────────────────────────────────────

func TestNotification_MarkAsRead_SetsReadAt(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})

	before := time.Now()
	updated, err := svc.MarkAsRead(userCtx("res-1"), n.ID)
	after := time.Now()
	if err != nil {
		t.Fatal(err)
	}

	if updated.ReadAt == nil {
		t.Fatal("expected ReadAt to be set after MarkAsRead")
	}
	if updated.ReadAt.Before(before) || updated.ReadAt.After(after) {
		t.Errorf("ReadAt %v outside expected window [%v, %v]", updated.ReadAt, before, after)
	}
}

func TestNotification_MarkAsRead_IdempotentDoesNotUpdateReadAt(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})
	ctx := userCtx("res-1")

	first, err := svc.MarkAsRead(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
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
	created := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})

	for _, ctx := range []context.Context{adminCtx(), userCtx("res-1")} {
		found, err := svc.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if found.ID != created.ID {
			t.Errorf("expected ID %q, got %q", created.ID, found.ID)
		}
	}
}

func TestNotification_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(adminCtx(), "ghost-id")
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestNotification_List_ReturnsAll(t *testing.T) {
	svc := newService()
	_ = mustCreate(t, svc, notification.CreateRequest{Message: "A", ResidentIDs: []string{"r1"}})
	_ = mustCreate(t, svc, notification.CreateRequest{Message: "B", ResidentIDs: []string{"r1"}})

	res, err := svc.List(adminCtx(), page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(res.Items) != 2 || res.Total != 2 {
		t.Errorf("expected 2 notifications, got %d (total %d)", len(res.Items), res.Total)
	}
}

func TestNotification_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	res, err := svc.List(adminCtx(), page.Default)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.Items == nil {
		t.Error("expected empty slice, not nil")
	}
}

func TestNotification_List_Paginates_NewestFirst(t *testing.T) {
	svc := newService()
	ids := make([]string, 0, 3)
	for _, msg := range []string{"1", "2", "3"} {
		ids = append(ids, mustCreate(t, svc, notification.CreateRequest{Message: msg, ResidentIDs: []string{"r1"}}).ID)
		time.Sleep(time.Millisecond) // distinct CreatedAt
	}

	res, err := svc.List(adminCtx(), page.Params{Page: 1, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || res.Page != 1 || res.PageSize != 2 || len(res.Items) != 2 {
		t.Fatalf("page 1: %+v", res)
	}
	if res.Items[0].ID != ids[2] || res.Items[1].ID != ids[1] {
		t.Errorf("expected newest first")
	}
	res, _ = svc.List(adminCtx(), page.Params{Page: 2, Size: 2})
	if res.Total != 3 || len(res.Items) != 1 || res.Items[0].ID != ids[0] {
		t.Errorf("page 2: %+v", res)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestNotification_Update_ChangesMessage(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, notification.CreateRequest{Message: "Mensagem original", ResidentIDs: []string{"r1"}})

	updated, err := svc.Update(adminCtx(), created.ID, notification.UpdateRequest{
		Message: "Mensagem atualizada",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Message != "Mensagem atualizada" {
		t.Errorf("expected 'Mensagem atualizada', got %q", updated.Message)
	}
}

// A list present in the body replaces the stored one, even when empty; an
// absent list is kept. At least one recipient must remain.
func TestNotification_Update_RecipientLists(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, notification.CreateRequest{Message: "m", ResidentIDs: []string{"r1"}, ShopOwnerIDs: []string{"s1"}})

	updated, err := svc.Update(adminCtx(), created.ID, notification.UpdateRequest{ShopOwnerIDs: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.ShopOwnerIDs) != 0 || len(updated.ResidentIDs) != 1 {
		t.Fatalf("[] must clear the shop owners and keep the residents, got %+v", updated)
	}
	if stored, _ := svc.GetByID(adminCtx(), created.ID); len(stored.ShopOwnerIDs) != 0 {
		t.Errorf("cleared list not persisted: %+v", stored)
	}

	if _, err := svc.Update(adminCtx(), created.ID, notification.UpdateRequest{ResidentIDs: []string{}}); !errors.Is(err, notification.ErrValidation) {
		t.Errorf("removing the last recipient: expected ErrValidation, got %v", err)
	}
}

func TestNotification_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(adminCtx(), "ghost-id", notification.UpdateRequest{Message: "X"})
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestNotification_Delete_Existing_Succeeds(t *testing.T) {
	svc := newService()
	created := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})

	if err := svc.Delete(adminCtx(), created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(adminCtx(), created.ID)
	if !notification.IsNotFoundError(err) {
		t.Error("expected notification to be deleted")
	}
}

func TestNotification_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(adminCtx(), "ghost-id")
	if !notification.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Security ──────────────────────────────────────────────────────────────────

func TestNotification_Security_UserCannotManage(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})
	ctx := userCtx("res-1") // even a recipient may not manage it

	if _, err := svc.Create(ctx, notification.CreateRequest{Message: "X", ResidentIDs: []string{"r1"}}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Create: expected ErrForbidden, got %v", err)
	}
	if _, err := svc.Update(ctx, n.ID, notification.UpdateRequest{Message: "X"}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Update: expected ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, n.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Delete: expected ErrForbidden, got %v", err)
	}
	got, err := svc.GetByID(adminCtx(), n.ID)
	if err != nil || got.Message != "Aviso" {
		t.Errorf("notification must be untouched: %+v (%v)", got, err)
	}
}

func TestNotification_Security_NotAddressedUserSeesNothing(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Para r1", ResidentIDs: []string{"r1"}})
	shop := mustCreate(t, svc, notification.CreateRequest{Message: "Para s1", ShopOwnerIDs: []string{"s1"}})
	ctx := userCtx("outsider")

	if _, err := svc.GetByID(ctx, n.ID); !notification.IsNotFoundError(err) {
		t.Errorf("GetByID: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.MarkAsRead(ctx, n.ID); !notification.IsNotFoundError(err) {
		t.Errorf("MarkAsRead: expected ErrNotFound, got %v", err)
	}
	res, err := svc.List(ctx, page.Default)
	if err != nil || len(res.Items) != 0 || res.Total != 0 {
		t.Errorf("List: expected empty, got %+v (%v)", res, err)
	}

	// shop owner sees only the one addressed to their shop
	res, _ = svc.List(userCtx("shop-1"), page.Default)
	if len(res.Items) != 1 || res.Items[0].ID != shop.ID || res.Total != 1 {
		t.Errorf("shop owner List: %+v", res)
	}
	// the outsider's attempted read did not mark it for anyone
	if got, _ := svc.GetByID(userCtx("res-1"), n.ID); got.Read {
		t.Error("outsider must not have marked the notification as read")
	}
}

func TestNotification_Security_ReadStateIsPerRecipient(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})

	a, err := svc.MarkAsRead(userCtx("res-1"), n.ID)
	if err != nil || !a.Read {
		t.Fatalf("A mark read: %+v (%v)", a, err)
	}
	if got, _ := svc.GetByID(userCtx("res-1"), n.ID); !got.Read || got.ReadAt == nil {
		t.Errorf("A must see read=true: %+v", got)
	}
	if got, _ := svc.GetByID(userCtx("res-2"), n.ID); got.Read || got.ReadAt != nil {
		t.Errorf("B must still see read=false: %+v", got)
	}
	if got, _ := svc.GetByID(adminCtx(), n.ID); got.Read {
		t.Errorf("admin must still see read=false: %+v", got)
	}
	res, _ := svc.List(userCtx("res-2"), page.Default)
	if len(res.Items) != 1 || res.Items[0].Read {
		t.Errorf("B list must show unread: %+v", res)
	}
}

func TestNotification_Security_OtherTenantIsInvisible(t *testing.T) {
	svc := newService()
	n := mustCreate(t, svc, notification.CreateRequest{Message: "Aviso", ResidentIDs: []string{"r1"}})
	other := middleware.WithIdentity(tenant.WithCondominiumID(context.Background(), "condo-b"), "admin-b", []string{authz.RoleAdmin})

	if _, err := svc.GetByID(other, n.ID); !notification.IsNotFoundError(err) {
		t.Errorf("GetByID: expected ErrNotFound, got %v", err)
	}
	if _, err := svc.Update(other, n.ID, notification.UpdateRequest{Message: "X"}); !notification.IsNotFoundError(err) {
		t.Errorf("Update: expected ErrNotFound, got %v", err)
	}
	if err := svc.Delete(other, n.ID); !notification.IsNotFoundError(err) {
		t.Errorf("Delete: expected ErrNotFound, got %v", err)
	}
	if res, _ := svc.List(other, page.Default); len(res.Items) != 0 || res.Total != 0 {
		t.Errorf("List: expected empty, got %+v", res)
	}
}

func TestInMemoryRepository_MissingTenant(t *testing.T) {
	repo := notification.NewInMemoryRepository()
	ctx := context.Background()
	if err := repo.Save(ctx, &notification.Notification{Message: "x"}); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("Save: expected ErrMissing, got %v", err)
	}
	if _, _, err := repo.FindAll(ctx, authz.Scope{All: true}, "", page.Default); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("FindAll: expected ErrMissing, got %v", err)
	}
	if _, err := repo.FindByID(ctx, "x", ""); !errors.Is(err, tenant.ErrMissing) {
		t.Errorf("FindByID: expected ErrMissing, got %v", err)
	}
}
