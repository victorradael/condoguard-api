package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/victorradael/condoguard/api/internal/auth"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/password"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	secret := "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="
	jwtSvc := pkgjwt.NewService(secret)
	repo := auth.NewInMemoryRepository()
	svc := auth.NewService(repo, jwtSvc)
	return auth.NewHandler(svc)
}

func postJSON(handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ── POST /auth/register ───────────────────────────────────────────────────────

func TestRegister_Success_Returns201(t *testing.T) {
	h := newTestHandler(t)

	rec := postJSON(h, "/auth/register", map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "S3cr3t!Pass",
		"roles":    []string{"ROLE_USER"},
	})

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] == "" {
		t.Error("expected message in response body")
	}
}

func TestRegister_DuplicateEmail_Returns409(t *testing.T) {
	h := newTestHandler(t)

	payload := map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "S3cr3t!Pass",
		"roles":    []string{"ROLE_USER"},
	}
	postJSON(h, "/auth/register", payload)

	payload["username"] = "alice2"
	rec := postJSON(h, "/auth/register", payload)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestRegister_MissingEmail_Returns422(t *testing.T) {
	h := newTestHandler(t)

	rec := postJSON(h, "/auth/register", map[string]any{
		"username": "bob",
		"password": "S3cr3t!Pass",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestRegister_MissingPassword_Returns422(t *testing.T) {
	h := newTestHandler(t)

	rec := postJSON(h, "/auth/register", map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestRegister_PasswordIsHashed(t *testing.T) {
	secret := "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="
	jwtSvc := pkgjwt.NewService(secret)
	repo := auth.NewInMemoryRepository()
	svc := auth.NewService(repo, jwtSvc)
	h := auth.NewHandler(svc)

	postJSON(h, "/auth/register", map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "S3cr3t!Pass",
		"roles":    []string{"ROLE_USER"},
	})

	user, err := repo.FindByEmail(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("user not found: %v", err)
	}
	if user.Password == "S3cr3t!Pass" {
		t.Error("password must be hashed, not stored as plaintext")
	}
	if !password.Verify("S3cr3t!Pass", user.Password) {
		t.Error("stored hash does not match original password")
	}
}

// ── POST /auth/login ──────────────────────────────────────────────────────────

func TestLogin_ValidCredentials_Returns200WithToken(t *testing.T) {
	h := newTestHandler(t)

	postJSON(h, "/auth/register", map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "S3cr3t!Pass",
		"roles":    []string{"ROLE_USER"},
	})

	rec := postJSON(h, "/auth/login", map[string]any{
		"username": "alice",
		"password": "S3cr3t!Pass",
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)

	if resp["token"] == "" || resp["token"] == nil {
		t.Error("expected token in response")
	}
	if resp["roles"] == nil {
		t.Error("expected roles in response")
	}
}

func TestLogin_WrongPassword_Returns401(t *testing.T) {
	h := newTestHandler(t)

	postJSON(h, "/auth/register", map[string]any{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "S3cr3t!Pass",
		"roles":    []string{"ROLE_USER"},
	})

	rec := postJSON(h, "/auth/login", map[string]any{
		"username": "alice",
		"password": "wrongpassword",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLogin_UnknownUser_Returns401(t *testing.T) {
	h := newTestHandler(t)

	rec := postJSON(h, "/auth/login", map[string]any{
		"username": "ghost",
		"password": "doesntmatter",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLogin_MissingUsername_Returns422(t *testing.T) {
	h := newTestHandler(t)

	rec := postJSON(h, "/auth/login", map[string]any{
		"password": "S3cr3t!Pass",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

// ── Integration (requires MONGODB_URI env) ────────────────────────────────────

func TestIntegration_Register_Login_RoundTrip(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI not set — skipping integration test")
	}

	secret := os.Getenv("JWT_SECRET_KEY")
	if secret == "" {
		secret = "dGVzdC1zZWNyZXQta2V5LWZvci1pbnRlZ3JhdGlvbg=="
	}

	ctx := context.Background()
	mongoRepo, err := auth.NewMongoRepository(ctx, uri, "condoguard_test")
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	defer mongoRepo.Cleanup(ctx)

	jwtSvc := pkgjwt.NewService(secret)
	svc := auth.NewService(mongoRepo, jwtSvc)
	h := auth.NewHandler(svc)

	// register
	rec := postJSON(h, "/auth/register", map[string]any{
		"username": "integration-user",
		"email":    "integration@example.com",
		"password": "TestPass123!",
		"roles":    []string{"ROLE_USER"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}

	// login
	rec = postJSON(h, "/auth/login", map[string]any{
		"username": "integration-user",
		"password": "TestPass123!",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d — %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["token"] == "" || resp["token"] == nil {
		t.Error("expected token in login response")
	}
}
