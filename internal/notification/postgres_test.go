package notification_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/notification"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// saveUser stores a real user (notification_reads references users) and returns its ID.
func saveUser(t *testing.T, pool *pgxpool.Pool, condo, name string, roles ...string) string {
	t.Helper()
	ctx := tenant.WithCondominiumID(context.Background(), condo)
	u := &user.User{
		Username: name,
		Email:    strings.ToLower(name+"-"+uuid.NewString()[:8]) + "@exemplo.test",
		Password: "hash",
		Roles:    roles,
	}
	if err := user.NewPostgresRepository(pool).Save(ctx, u); err != nil {
		t.Fatalf("save user %s: %v", name, err)
	}
	return u.ID
}

func TestPostgresRepository_Notification(t *testing.T) {
	env := dbtest.NewEnv(t)
	pool := env.Pool
	repo := notification.NewPostgresRepository(pool)
	ctxA := tenant.WithCondominiumID(context.Background(), env.Condo("condo-a"))
	ctxB := tenant.WithCondominiumID(context.Background(), env.Condo("condo-b"))
	all := authz.Scope{All: true}

	userA := saveUser(t, pool, env.Condo("condo-a"), "alice", authz.RoleMorador)
	userB := saveUser(t, pool, env.Condo("condo-a"), "bob", authz.RoleMorador)

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	older := &notification.Notification{Message: "antiga", CreatedByID: "admin", CreatedAt: base, ResidentIDs: []string{"r1", "r2"}}
	newer := &notification.Notification{Message: "nova", CreatedByID: "admin", CreatedAt: base.Add(time.Hour), ShopOwnerIDs: []string{"s1"}}
	for _, n := range []*notification.Notification{older, newer} {
		if err := repo.Save(ctxA, n); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	t.Run("list newest first with non-nil recipient slices", func(t *testing.T) {
		list, total, err := repo.FindAll(ctxA, all, userA, page.Default)
		if err != nil || len(list) != 2 || total != 2 {
			t.Fatalf("expected 2, got %d/%d (%v)", len(list), total, err)
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

	t.Run("paging", func(t *testing.T) {
		list, total, err := repo.FindAll(ctxA, all, "", page.Params{Page: 1, Size: 1})
		if err != nil || total != 2 || len(list) != 1 || list[0].ID != newer.ID {
			t.Fatalf("page 1: %d/%d (%v)", len(list), total, err)
		}
		list, total, _ = repo.FindAll(ctxA, all, "", page.Params{Page: 2, Size: 1})
		if total != 2 || len(list) != 1 || list[0].ID != older.ID {
			t.Errorf("page 2: %d/%d", len(list), total)
		}
		list, total, _ = repo.FindAll(ctxA, all, "", page.Params{Page: 3, Size: 1})
		if total != 2 || len(list) != 0 {
			t.Errorf("page 3: %d/%d", len(list), total)
		}
	})

	t.Run("scope filter", func(t *testing.T) {
		cases := []struct {
			name  string
			units authz.Units
			want  []string
		}{
			{"resident r2", authz.Units{ResidentIDs: []string{"r2"}}, []string{older.ID}},
			{"shop s1", authz.Units{ShopOwnerIDs: []string{"s1"}}, []string{newer.ID}},
			{"both", authz.Units{ResidentIDs: []string{"r1"}, ShopOwnerIDs: []string{"s1"}}, []string{newer.ID, older.ID}},
			{"unaddressed", authz.Units{ResidentIDs: []string{"r9"}, ShopOwnerIDs: []string{"s9"}}, nil},
			{"no units", authz.Units{}, nil},
		}
		for _, c := range cases {
			list, total, err := repo.FindAll(ctxA, authz.Scope{Units: c.units}, "", page.Default)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if len(list) != len(c.want) || total != int64(len(c.want)) {
				t.Errorf("%s: expected %d, got %d/%d", c.name, len(c.want), len(list), total)
				continue
			}
			for i, id := range c.want {
				if list[i].ID != id {
					t.Errorf("%s: item %d = %s, want %s", c.name, i, list[i].ID, id)
				}
			}
		}
	})

	t.Run("read state is per user and idempotent", func(t *testing.T) {
		first, err := repo.MarkRead(ctxA, older.ID, userA)
		if err != nil || first.IsZero() {
			t.Fatalf("MarkRead: %v %v", first, err)
		}
		again, err := repo.MarkRead(ctxA, older.ID, userA)
		if err != nil || !again.Equal(first) {
			t.Errorf("MarkRead must be idempotent: first=%v again=%v (%v)", first, again, err)
		}

		a, err := repo.FindByID(ctxA, older.ID, userA)
		if err != nil || !a.Read || a.ReadAt == nil || !a.ReadAt.Equal(first) {
			t.Errorf("A must see read: %+v (%v)", a, err)
		}
		b, err := repo.FindByID(ctxA, older.ID, userB)
		if err != nil || b.Read || b.ReadAt != nil {
			t.Errorf("B must still see unread: %+v (%v)", b, err)
		}
		if anon, _ := repo.FindByID(ctxA, older.ID, "not-a-uuid"); anon.Read {
			t.Error("non-UUID reader must have no read state")
		}

		listA, _, _ := repo.FindAll(ctxA, all, userA, page.Default)
		listB, _, _ := repo.FindAll(ctxA, all, userB, page.Default)
		if !listA[1].Read || listA[0].Read || listB[1].Read || listB[0].Read {
			t.Errorf("list read state: A=[%v %v] B=[%v %v]", listA[0].Read, listA[1].Read, listB[0].Read, listB[1].Read)
		}

		bAt, err := repo.MarkRead(ctxA, older.ID, userB)
		if err != nil || bAt.Before(first) {
			t.Errorf("B MarkRead: %v (%v)", bAt, err)
		}
		if a, _ := repo.FindByID(ctxA, older.ID, userA); !a.ReadAt.Equal(first) {
			t.Errorf("B reading must not change A's readAt: %v", a.ReadAt)
		}
	})

	t.Run("update changes message and recipients only", func(t *testing.T) {
		n, _ := repo.FindByID(ctxA, older.ID, userA)
		n.Message, n.ResidentIDs = "editada", []string{"r3"}
		if err := repo.Update(ctxA, n); err != nil {
			t.Fatal(err)
		}
		got, err := repo.FindByID(ctxA, older.ID, userA)
		if err != nil {
			t.Fatal(err)
		}
		if got.Message != "editada" || len(got.ResidentIDs) != 1 || got.ResidentIDs[0] != "r3" ||
			!got.CreatedAt.Equal(base) || got.CreatedByID != "admin" || !got.Read {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("mark read on unknown notification", func(t *testing.T) {
		if _, err := repo.MarkRead(ctxA, uuid.NewString(), userA); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("unknown id: expected ErrNotFound, got %v", err)
		}
		if _, err := repo.MarkRead(ctxA, "ghost-id", userA); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("malformed id: expected ErrNotFound, got %v", err)
		}
		if _, err := repo.MarkRead(ctxA, newer.ID, "not-a-uuid"); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("malformed user: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("tenant isolation", func(t *testing.T) {
		if _, err := repo.FindByID(ctxB, older.ID, userA); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("FindByID: expected ErrNotFound, got %v", err)
		}
		if list, total, _ := repo.FindAll(ctxB, all, userA, page.Default); len(list) != 0 || total != 0 {
			t.Errorf("FindAll: expected empty, got %d/%d", len(list), total)
		}
		if err := repo.Update(ctxB, older); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("Update: expected ErrNotFound, got %v", err)
		}
		if _, err := repo.MarkRead(ctxB, newer.ID, userA); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("MarkRead: expected ErrNotFound, got %v", err)
		}
		if n, _ := repo.FindByID(ctxA, newer.ID, userA); n.Read {
			t.Error("MarkRead from another tenant must not record a read")
		}
		if err := repo.Delete(ctxB, older.ID); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("Delete: expected ErrNotFound, got %v", err)
		}
		if _, _, err := repo.FindAll(context.Background(), all, "", page.Default); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("missing tenant: expected ErrMissing, got %v", err)
		}
		if _, err := repo.MarkRead(context.Background(), older.ID, userA); !errors.Is(err, tenant.ErrMissing) {
			t.Errorf("missing tenant MarkRead: expected ErrMissing, got %v", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := repo.Delete(ctxA, newer.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctxA, newer.ID); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("second delete: expected ErrNotFound, got %v", err)
		}
		// deleting a read notification cascades its read rows
		if err := repo.Delete(ctxA, older.ID); err != nil {
			t.Fatalf("delete read notification: %v", err)
		}
		if _, err := repo.MarkRead(ctxA, older.ID, userA); !errors.Is(err, notification.ErrNotFound) {
			t.Errorf("MarkRead after delete: expected ErrNotFound, got %v", err)
		}
	})
}
