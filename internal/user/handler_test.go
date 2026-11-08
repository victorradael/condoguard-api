package user_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/tenant"

	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/user"
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

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func TestIntegration_Users_CRUD(t *testing.T) {
	pool := dbtest.NewPool(t)
	router := tenant.Middleware("condo-test")(user.NewHandler(user.NewService(user.NewPostgresRepository(pool)), middleware.Authenticate(pkgjwt.NewService(handlerSecret))))

	crudRoundTrip(t, router, adminToken(t), "/users",
		map[string]any{"username": "pg-user", "email": "pg-user@example.com", "password": "S3cr3t!Pass", "roles": []string{"ROLE_USER"}},
		map[string]any{"username": "pg-user-renamed", "email": "ignored@example.com"},
		func(_, updated map[string]any) {
			if updated["username"] != "pg-user-renamed" {
				t.Errorf("expected renamed username, got %v", updated["username"])
			}
			if updated["email"] != "pg-user@example.com" {
				t.Errorf("email must be immutable, got %v", updated["email"])
			}
		})
}

// crudRoundTrip drives create → get → list → update → delete through the HTTP
// handler backed by the PostgreSQL repository.
func crudRoundTrip(t *testing.T, router http.Handler, tok, base string, create, update map[string]any, check func(created, updated map[string]any)) {
	t.Helper()
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, authRequest(method, path, tok, body))
		return rec
	}

	rec := do(http.MethodPost, base, create)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&created)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("create: expected id")
	}

	if rec = do(http.MethodGet, base+"/"+id, nil); rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}
	if rec = do(http.MethodGet, base, nil); rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var list []map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("list: expected 1 item, got %d", len(list))
	}

	rec = do(http.MethodPut, base+"/"+id, update)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	if rec = do(http.MethodGet, base+"/"+id, nil); rec.Code != http.StatusOK {
		t.Fatalf("get after update: expected 200, got %d", rec.Code)
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	check(created, updated)

	if rec = do(http.MethodDelete, base+"/"+id, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rec.Code)
	}
	if rec = do(http.MethodGet, base+"/"+id, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", rec.Code)
	}
}
