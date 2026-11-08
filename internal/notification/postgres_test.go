package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/notification"
	"github.com/victorradael/condoguard/api/internal/tenant"
)

func TestPostgresRepository_Notification(t *testing.T) {
	repo := notification.NewPostgresRepository(dbtest.NewPool(t))
	ctxA := tenant.WithCondominiumID(context.Background(), "condo-a")
	ctxB := tenant.WithCondominiumID(context.Background(), "condo-b")

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	older := &notification.Notification{Message: "antiga", CreatedByID: "admin", CreatedAt: base, ResidentIDs: []string{"r1", "r2"}}
	newer := &notification.Notification{Message: "nova", CreatedByID: "admin", CreatedAt: base.Add(time.Hour), ShopOwnerIDs: []string{"s1"}}
	for _, n := range []*notification.Notification{older, newer} {
		if err := repo.Save(ctxA, n); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	t.Run("list newest first with non-nil recipient slices", func(t *testing.T) {
		list, err := repo.FindAll(ctxA)
		if err != nil || len(list) != 2 {
			t.Fatalf("expected 2, got %d (%v)", len(list), err)
		}
		if list[0].ID != newer.ID || list[1].ID != older.ID {
			t.Errorf("expected newest first")
		}
		if list[0].ResidentIDs == nil || len(list[0].ResidentIDs) != 0 || len(list[0].ShopOwnerIDs) != 1 {
			t.Errorf("recipients: %+v", list[0])
		}
		if len(list[1].ResidentIDs) != 2 || list[1].ResidentIDs[1] != "r2" || list[1].ShopOwnerIDs == nil {
			t.Errorf("recipients: %+v", list[1])
		}
		if list[0].Read || list[0].ReadAt != nil {
			t.Error("new notification must be unread")
		}
	})

	t.Run("mark as read persists readAt", func(t *testing.T) {
		n, _ := repo.FindByID(ctxA, older.ID)
		readAt := base.Add(2 * time.Hour)
		n.Read, n.ReadAt, n.Message = true, &readAt, "editada"
		if err := repo.Update(ctxA, n); err != nil {
			t.Fatal(err)
		}
		got, err := repo.FindByID(ctxA, older.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Read || got.ReadAt == nil || !got.ReadAt.Equal(readAt) || got.Message != "editada" || !got.CreatedAt.Equal(base) {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		if _, err := repo.FindByID(ctxB, older.ID); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("FindByID: expected ErrNotFound, got %v", err)
		}
		if list, _ := repo.FindAll(ctxB); len(list) != 0 {
			t.Errorf("FindAll: expected empty, got %d", len(list))
		}
		if err := repo.Update(ctxB, older); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("Update: expected ErrNotFound, got %v", err)
		}
		if err := repo.Delete(ctxB, older.ID); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("Delete: expected ErrNotFound, got %v", err)
		}
		if _, err := repo.FindAll(context.Background()); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("missing tenant: expected ErrMissing, got %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := repo.Delete(ctxA, newer.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctxA, newer.ID); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("second delete: expected ErrNotFound, got %v", err)
		}
	})
}
