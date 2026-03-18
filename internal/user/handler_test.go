package user_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/victorradael/condoguard/api/internal/user"
	"github.com/victorradael/condoguard/api/internal/middleware"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="

// ── test helpers ──────────────────────────────────────────────────────────────

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	repo := user.NewInMemoryRepository()
	svc := user.NewService(repo)
	jwtSvc := pkgjwt.NewService(handlerSecret)
	return user.NewHandler(svc, middleware.Authenticate(jwtSvc))
}

func adminToken(t *testing.T) string {
	t.Helper()
	svc := pkgjwt.NewService(handlerSecret)
	tok, err := svc.GenerateToken("admin-1", []string{"ROLE_ADMIN"})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func userToken(t *testing.T) string {
	t.Helper()
	svc := pkgjwt.NewService(handlerSecret)
	tok, err := svc.GenerateToken("user-1", []string{"ROLE_USER"})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func authRequest(method, path, token string, body any) *http.Request {
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestUsers_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodGet, "/users", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestUsers_NonAdminToken_Returns403(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodGet, "/users", userToken(t), nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// ── GET /users ────────────────────────────────────────────────────────────────

func TestUsers_List_EmptyDatabase_Returns200WithEmptyArray(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodGet, "/users", adminToken(t), nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp []any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("expected empty array, got %v", resp)
	}
}

// ── POST /users ───────────────────────────────────────────────────────────────

func TestUsers_Create_Returns201WithUser(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
		"password": "S3cr3t!",
		"roles":    []string{"ROLE_USER"},
	}))

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — %s", rec.Code, rec.Body.String())
	}

	var u map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&u)
	if u["id"] == "" || u["id"] == nil {
		t.Error("expected id in response")
	}
	if u["password"] != nil && u["password"] != "" {
		t.Error("password must not be present in response")
	}
}

func TestUsers_Create_DuplicateEmail_Returns409(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	router.ServeHTTP(httptest.NewRecorder(), authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
		"password": "S3cr3t!",
	}))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "bob2",
		"email":    "bob@example.com",
		"password": "S3cr3t!",
	}))

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestUsers_Create_MissingEmail_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
		"username": "bob",
		"password": "S3cr3t!",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

// ── GET /users/{id} ───────────────────────────────────────────────────────────

func TestUsers_GetByID_ExistingUser_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "carol",
		"email":    "carol@example.com",
		"password": "S3cr3t!",
	}))
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id := fmt.Sprintf("%v", created["id"])

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, authRequest(http.MethodGet, "/users/"+id, tok, nil))

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec2.Code)
	}
}

func TestUsers_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodGet, "/users/ghost-id", adminToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /users/{id} ───────────────────────────────────────────────────────────

func TestUsers_Update_Returns200WithUpdatedUser(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "dave",
		"email":    "dave@example.com",
		"password": "S3cr3t!",
	}))
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id := fmt.Sprintf("%v", created["id"])

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, authRequest(http.MethodPut, "/users/"+id, tok, map[string]any{
		"username": "dave-updated",
	}))

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec2.Code, rec2.Body.String())
	}

	var updated map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&updated)
	if updated["username"] != "dave-updated" {
		t.Errorf("expected username 'dave-updated', got %v", updated["username"])
	}
}

func TestUsers_Update_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPut, "/users/ghost-id", adminToken(t), map[string]any{
		"username": "x",
	}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUsers_Update_EmailIsIgnored(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "eve",
		"email":    "eve@example.com",
		"password": "S3cr3t!",
	}))
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id := fmt.Sprintf("%v", created["id"])

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, authRequest(http.MethodPut, "/users/"+id, tok, map[string]any{
		"username": "eve",
		"email":    "hacked@example.com",
	}))

	var updated map[string]any
	_ = json.NewDecoder(rec2.Body).Decode(&updated)
	if updated["email"] != "eve@example.com" {
		t.Errorf("email must be immutable; got %v", updated["email"])
	}
}

// ── DELETE /users/{id} ────────────────────────────────────────────────────────

func TestUsers_Delete_ExistingUser_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "frank",
		"email":    "frank@example.com",
		"password": "S3cr3t!",
	}))
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id := fmt.Sprintf("%v", created["id"])

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, authRequest(http.MethodDelete, "/users/"+id, tok, nil))

	if rec2.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec2.Code)
	}
}

func TestUsers_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authRequest(http.MethodDelete, "/users/ghost-id", adminToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Integration ───────────────────────────────────────────────────────────────

func TestIntegration_Users_CRUD(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI not set — skipping integration test")
	}
	// Integration test body omitted until Mongo repo for user is wired.
	// Will be completed in the same pattern as auth integration test.
	t.Log("integration scaffold ready — wire MongoRepository to proceed")
}
