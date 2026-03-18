package user_test

import (
	"context"
	"testing"

	"github.com/victorradael/condoguard/api/internal/user"
)

func newService() *user.Service {
	return user.NewService(user.NewInMemoryRepository())
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestUser_Create_Success(t *testing.T) {
	svc := newService()

	u, err := svc.Create(context.Background(), user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
		Roles:    []string{"ROLE_USER"},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if u.ID == "" {
		t.Error("expected non-empty ID")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %q", u.Email)
	}
}

func TestUser_Create_MissingEmail_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), user.CreateRequest{
		Username: "alice",
		Password: "S3cr3t!",
	})

	if !user.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestUser_Create_MissingUsername_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), user.CreateRequest{
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	if !user.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestUser_Create_MissingPassword_ReturnsValidationError(t *testing.T) {
	svc := newService()

	_, err := svc.Create(context.Background(), user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
	})

	if !user.IsValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestUser_Create_DuplicateEmail_ReturnsDuplicateError(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	_, err := svc.Create(ctx, user.CreateRequest{
		Username: "alice2",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	if !user.IsDuplicateError(err) {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestUser_Create_PasswordIsHashed(t *testing.T) {
	repo := user.NewInMemoryRepository()
	svc := user.NewService(repo)
	ctx := context.Background()

	created, _ := svc.Create(ctx, user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	stored, _ := repo.FindByID(ctx, created.ID)
	if stored.Password == "S3cr3t!" {
		t.Error("password must be hashed, not stored as plaintext")
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestUser_GetByID_ExistingUser_ReturnsUser(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	found, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, found.ID)
	}
}

func TestUser_GetByID_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.GetByID(context.Background(), "nonexistent-id")
	if !user.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestUser_List_ReturnsAllUsers(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	_, _ = svc.Create(ctx, user.CreateRequest{Username: "u1", Email: "u1@x.com", Password: "pass"})
	_, _ = svc.Create(ctx, user.CreateRequest{Username: "u2", Email: "u2@x.com", Password: "pass"})

	users, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestUser_List_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newService()

	users, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if users == nil {
		t.Error("expected empty slice, got nil")
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUser_Update_ChangesUsernameAndRoles(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	updated, err := svc.Update(ctx, created.ID, user.UpdateRequest{
		Username: "alice-updated",
		Roles:    []string{"ROLE_ADMIN"},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Username != "alice-updated" {
		t.Errorf("expected username 'alice-updated', got %q", updated.Username)
	}
	if len(updated.Roles) != 1 || updated.Roles[0] != "ROLE_ADMIN" {
		t.Errorf("expected roles [ROLE_ADMIN], got %v", updated.Roles)
	}
}

func TestUser_Update_EmailIsImmutable(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	updated, err := svc.Update(ctx, created.ID, user.UpdateRequest{
		Username: "alice",
		Email:    "newemail@example.com", // must be ignored
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Email != "alice@example.com" {
		t.Errorf("email must be immutable; expected 'alice@example.com', got %q", updated.Email)
	}
}

func TestUser_Update_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	_, err := svc.Update(context.Background(), "ghost-id", user.UpdateRequest{Username: "x"})
	if !user.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestUser_Delete_ExistingUser_Succeeds(t *testing.T) {
	svc := newService()
	ctx := context.Background()

	created, _ := svc.Create(ctx, user.CreateRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "S3cr3t!",
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if !user.IsNotFoundError(err) {
		t.Error("expected user to be deleted")
	}
}

func TestUser_Delete_NonExistent_ReturnsNotFoundError(t *testing.T) {
	svc := newService()

	err := svc.Delete(context.Background(), "ghost-id")
	if !user.IsNotFoundError(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}
