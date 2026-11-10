package user_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/user"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const (
	handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"
	condoA        = dbtest.CondoA
	condoB        = dbtest.CondoB
	strongPass    = "Str0ng-Passw0rd"
)

var jwtSvc = pkgjwt.MustNewService(handlerSecret)

// ── test helpers ──────────────────────────────────────────────────────────────

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	return newRouterWithRepo(user.NewInMemoryRepository())
}

func newRouterWithRepo(repo user.Repository) http.Handler {
	return user.NewHandler(user.NewService(repo), middleware.Authenticate(jwtSvc))
}

func newPostgresRouter(t *testing.T) http.Handler {
	t.Helper()
	return newRouterWithRepo(user.NewPostgresRepository(dbtest.NewPool(t, dbtest.CondoA, dbtest.CondoB)))
}

func tokenFor(t *testing.T, userID, condoID string, roles ...string) string {
	t.Helper()
	tok, err := jwtSvc.GenerateToken(userID, condoID, roles)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func adminToken(t *testing.T) string {
	t.Helper()
	return tokenFor(t, "admin-1", condoA, "sindico")
}

func userToken(t *testing.T) string {
	t.Helper()
	return tokenFor(t, "user-1", condoA, "morador")
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

func serve(router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// createUser creates a user through POST /users and returns the decoded body.
func createUser(t *testing.T, router http.Handler, tok string, body map[string]any) map[string]any {
	t.Helper()
	rec := serve(router, authRequest(http.MethodPost, "/users", tok, body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("create: decode: %v", err)
	}
	return created
}

type listResponse struct {
	Items    []map[string]any `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) listResponse {
	t.Helper()
	var resp listResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return resp
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestUsers_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	for _, path := range []string{"/users", "/users/me"} {
		if rec := serve(router, authRequest(http.MethodGet, path, "", nil)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", path, rec.Code)
		}
	}
}

func TestUsers_NonAdminToken_Returns403(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodGet, "/users", userToken(t), nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// ── GET /users ────────────────────────────────────────────────────────────────

func TestUsers_List_EmptyDatabase_Returns200WithEmptyPage(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodGet, "/users", adminToken(t), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("items must be an empty JSON array, got %s", rec.Body.String())
	}
	resp := decodeList(t, rec)
	if resp.Total != 0 || resp.Page != 1 || resp.PageSize != 20 {
		t.Errorf("unexpected empty page envelope: %+v", resp)
	}
}

// ── POST /users ───────────────────────────────────────────────────────────────

func TestUsers_Create_Returns201WithUser(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
		"password": strongPass,
		"roles":    []string{"morador"},
	}))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — %s", rec.Code, rec.Body.String())
	}

	var u map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&u)
	if u["id"] == "" || u["id"] == nil {
		t.Error("expected id in response")
	}
	if _, ok := u["password"]; ok {
		t.Error("password must not be present in response")
	}
}

func TestUsers_Create_DefaultsToRoleUser_AndNormalisesEmail(t *testing.T) {
	router := newTestRouter(t)
	u := createUser(t, router, adminToken(t), map[string]any{
		"username": "bob",
		"email":    "  Bob@Example.COM ",
		"password": strongPass,
	})
	if u["email"] != "bob@example.com" {
		t.Errorf("expected normalised email, got %v", u["email"])
	}
	roles, _ := u["roles"].([]any)
	if len(roles) != 1 || roles[0] != "morador" {
		t.Errorf("expected default roles [morador], got %v", u["roles"])
	}
}

func TestUsers_Create_DuplicateEmail_Returns409(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	createUser(t, router, tok, map[string]any{
		"username": "bob",
		"email":    "bob@example.com",
		"password": strongPass,
	})

	rec := serve(router, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "bob2",
		"email":    "bob@example.com",
		"password": strongPass,
	}))

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestUsers_Create_MissingEmail_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
		"username": "bob",
		"password": strongPass,
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestUsers_Create_InvalidJSON_Returns400(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	if rec := serve(router, req); rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ── GET /users/{id} ───────────────────────────────────────────────────────────

func TestUsers_GetByID_ExistingUser_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	created := createUser(t, router, tok, map[string]any{
		"username": "carol",
		"email":    "carol@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	if rec := serve(router, authRequest(http.MethodGet, "/users/"+id, tok, nil)); rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestUsers_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodGet, "/users/ghost-id", adminToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /users/{id} ───────────────────────────────────────────────────────────

func TestUsers_Update_Returns200WithUpdatedUser(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	created := createUser(t, router, tok, map[string]any{
		"username": "dave",
		"email":    "dave@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	rec := serve(router, authRequest(http.MethodPut, "/users/"+id, tok, map[string]any{
		"username": "dave-updated",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}

	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["username"] != "dave-updated" {
		t.Errorf("expected username 'dave-updated', got %v", updated["username"])
	}
}

func TestUsers_Update_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodPut, "/users/ghost-id", adminToken(t), map[string]any{
		"username": "x",
	}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestUsers_Update_EmailIsIgnored(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	created := createUser(t, router, tok, map[string]any{
		"username": "eve",
		"email":    "eve@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	rec := serve(router, authRequest(http.MethodPut, "/users/"+id, tok, map[string]any{
		"username": "eve",
		"email":    "hacked@example.com",
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}

	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["email"] != "eve@example.com" {
		t.Errorf("email must be immutable; got %v", updated["email"])
	}
}

// ── DELETE /users/{id} ────────────────────────────────────────────────────────

func TestUsers_Delete_ExistingUser_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	created := createUser(t, router, tok, map[string]any{
		"username": "frank",
		"email":    "frank@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	if rec := serve(router, authRequest(http.MethodDelete, "/users/"+id, tok, nil)); rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestUsers_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodDelete, "/users/ghost-id", adminToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Security: role enforcement ────────────────────────────────────────────────

func TestHandler_Security_RoleUserForbiddenOnAdminRoutes(t *testing.T) {
	router := newTestRouter(t)
	victim := createUser(t, router, adminToken(t), map[string]any{
		"username": "victim",
		"email":    "victim@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", victim["id"])
	tok := userToken(t)

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/users", nil},
		{http.MethodPost, "/users", map[string]any{"username": "x", "email": "x@example.com", "password": strongPass, "roles": []string{"sindico"}}},
		{http.MethodGet, "/users/" + id, nil},
		{http.MethodPut, "/users/" + id, map[string]any{"roles": []string{"sindico"}}},
		{http.MethodDelete, "/users/" + id, nil},
		{http.MethodPut, "/users/me", map[string]any{"roles": []string{"sindico"}}},
		{http.MethodDelete, "/users/me", nil},
	}
	for _, c := range cases {
		if rec := serve(router, authRequest(c.method, c.path, tok, c.body)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403, got %d — %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}

	// Nothing changed: the victim still exists with its original roles, and no user was created.
	rec := serve(router, authRequest(http.MethodGet, "/users", adminToken(t), nil))
	list := decodeList(t, rec)
	if list.Total != 1 {
		t.Fatalf("expected exactly 1 user after forbidden calls, got %d", list.Total)
	}
	roles, _ := list.Items[0]["roles"].([]any)
	if len(roles) != 1 || roles[0] != "morador" {
		t.Errorf("victim roles must be unchanged, got %v", list.Items[0]["roles"])
	}
}

func TestHandler_Security_TokenWithoutRolesForbidden(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodGet, "/users", tokenFor(t, "u-1", condoA), nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestHandler_Security_MeReturnsOwnUserForRoleUser(t *testing.T) {
	router := newTestRouter(t)
	created := createUser(t, router, adminToken(t), map[string]any{
		"username": "maria",
		"email":    "maria@example.com",
		"password": strongPass,
	})
	createUser(t, router, adminToken(t), map[string]any{
		"username": "other",
		"email":    "other@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	rec := serve(router, authRequest(http.MethodGet, "/users/me", tokenFor(t, id, condoA, "morador"), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var me map[string]any
	if err := json.Unmarshal([]byte(body), &me); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if me["id"] != id || me["username"] != "maria" || me["email"] != "maria@example.com" {
		t.Errorf("unexpected /users/me body: %v", me)
	}
	roles, _ := me["roles"].([]any)
	if len(roles) != 1 || roles[0] != "morador" {
		t.Errorf("unexpected roles: %v", me["roles"])
	}
	for _, k := range []string{"password", "passwordHash", "password_hash"} {
		if _, ok := me[k]; ok {
			t.Errorf("response must not contain %q", k)
		}
	}
	if strings.Contains(body, "$2a$") || strings.Contains(body, strongPass) {
		t.Errorf("response leaks password material: %s", body)
	}
}

func TestHandler_Security_MeForAdminReturnsOwnUser(t *testing.T) {
	router := newTestRouter(t)
	created := createUser(t, router, adminToken(t), map[string]any{
		"username": "boss",
		"email":    "boss@example.com",
		"password": strongPass,
		"roles":    []string{"sindico"},
	})
	id := fmt.Sprintf("%v", created["id"])
	rec := serve(router, authRequest(http.MethodGet, "/users/me", tokenFor(t, id, condoA, "sindico"), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var me map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&me)
	if me["id"] != id {
		t.Errorf("expected own id %s, got %v", id, me["id"])
	}
}

func TestHandler_Security_MeForDeletedAccountReturns401(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, authRequest(http.MethodGet, "/users/me", tokenFor(t, "gone-user", condoA, "morador"), nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── Security: input validation ────────────────────────────────────────────────

func TestHandler_Security_CreateWithUnknownRoleReturns422(t *testing.T) {
	router := newTestRouter(t)
	for _, roles := range [][]string{{"ROLE_SUPERUSER"}, {"morador", "ROLE_SUPERUSER"}, {"role_admin"}, {""}} {
		rec := serve(router, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
			"username": "eve",
			"email":    "eve@example.com",
			"password": strongPass,
			"roles":    roles,
		}))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("roles %v: expected 422, got %d — %s", roles, rec.Code, rec.Body.String())
		}
	}
}

func TestHandler_Security_UpdateWithUnknownRoleReturns422(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	created := createUser(t, router, tok, map[string]any{
		"username": "eve",
		"email":    "eve@example.com",
		"password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	rec := serve(router, authRequest(http.MethodPut, "/users/"+id, tok, map[string]any{"roles": []string{"ROLE_SUPERUSER"}}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d — %s", rec.Code, rec.Body.String())
	}
	rec = serve(router, authRequest(http.MethodGet, "/users/"+id, tok, nil))
	var got map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&got)
	roles, _ := got["roles"].([]any)
	if len(roles) != 1 || roles[0] != "morador" {
		t.Errorf("roles must be unchanged after rejected update, got %v", got["roles"])
	}
}

func TestHandler_Security_CreateWithWeakPasswordReturns422(t *testing.T) {
	router := newTestRouter(t)
	weak := map[string]string{
		"empty":             "",
		"too short":         "Ab1-xyz",
		"no digits":         "OnlyLetters-Here",
		"no letters":        "1234567890-12",
		"common":            "Password123",
		"contains username": "Xjoaosilva12",
		"contains email":    "zz-joao.s99-zz",
		"over 72 bytes":     strings.Repeat("a1", 37),
	}
	for name, pw := range weak {
		rec := serve(router, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
			"username": "joaosilva",
			"email":    "joao.s99@example.com",
			"password": pw,
		}))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s (%q): expected 422, got %d — %s", name, pw, rec.Code, rec.Body.String())
		}
	}
}

func TestHandler_Security_CreateWithInvalidEmailReturns422(t *testing.T) {
	router := newTestRouter(t)
	for _, e := range []string{"not-an-email", "ana@localhost", "Ana <ana@x.io>", "ana@x.io, bia@x.io"} {
		rec := serve(router, authRequest(http.MethodPost, "/users", adminToken(t), map[string]any{
			"username": "ana",
			"email":    e,
			"password": strongPass,
		}))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%q: expected 422, got %d", e, rec.Code)
		}
	}
}

func TestHandler_Security_DuplicateEmailIsCaseInsensitive(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	createUser(t, router, tok, map[string]any{"username": "ana", "email": "Ana@X.io", "password": strongPass})

	rec := serve(router, authRequest(http.MethodPost, "/users", tok, map[string]any{
		"username": "ana2", "email": " ana@x.io ", "password": strongPass,
	}))
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d — %s", rec.Code, rec.Body.String())
	}
}

// ── Pagination ────────────────────────────────────────────────────────────────

func TestHandler_List_Pagination(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	for i := 1; i <= 5; i++ {
		createUser(t, router, tok, map[string]any{
			"username": fmt.Sprintf("user%d", i),
			"email":    fmt.Sprintf("user%d@example.com", i),
			"password": strongPass,
		})
	}

	t.Run("defaults", func(t *testing.T) {
		resp := decodeList(t, serve(router, authRequest(http.MethodGet, "/users", tok, nil)))
		if resp.Page != 1 || resp.PageSize != 20 || resp.Total != 5 || len(resp.Items) != 5 {
			t.Errorf("unexpected default page: page=%d size=%d total=%d items=%d", resp.Page, resp.PageSize, resp.Total, len(resp.Items))
		}
	})

	t.Run("second page", func(t *testing.T) {
		rec := serve(router, authRequest(http.MethodGet, "/users?page=2&pageSize=2", tok, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		resp := decodeList(t, rec)
		if resp.Page != 2 || resp.PageSize != 2 || resp.Total != 5 || len(resp.Items) != 2 {
			t.Fatalf("unexpected page: %+v", resp)
		}
		if resp.Items[0]["username"] != "user3" || resp.Items[1]["username"] != "user4" {
			t.Errorf("expected user3,user4 in insertion order, got %v,%v", resp.Items[0]["username"], resp.Items[1]["username"])
		}
		for _, it := range resp.Items {
			if _, ok := it["password"]; ok {
				t.Error("list items must not contain password")
			}
		}
	})

	t.Run("last partial page", func(t *testing.T) {
		resp := decodeList(t, serve(router, authRequest(http.MethodGet, "/users?page=3&pageSize=2", tok, nil)))
		if resp.Total != 5 || len(resp.Items) != 1 || resp.Items[0]["username"] != "user5" {
			t.Errorf("unexpected last page: %+v", resp)
		}
	})

	t.Run("past the end", func(t *testing.T) {
		rec := serve(router, authRequest(http.MethodGet, "/users?page=10&pageSize=2", tok, nil))
		if !strings.Contains(rec.Body.String(), `"items":[]`) {
			t.Errorf("expected empty items array, got %s", rec.Body.String())
		}
	})

	t.Run("max page size accepted", func(t *testing.T) {
		if rec := serve(router, authRequest(http.MethodGet, "/users?pageSize=100", tok, nil)); rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid parameters return 400", func(t *testing.T) {
		for _, q := range []string{"page=0", "page=-1", "page=abc", "pageSize=0", "pageSize=101", "pageSize=x", "page=99999999999"} {
			if rec := serve(router, authRequest(http.MethodGet, "/users?"+q, tok, nil)); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400, got %d", q, rec.Code)
			}
		}
	})
}

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func TestIntegration_Users_CRUD(t *testing.T) {
	router := newPostgresRouter(t)

	crudRoundTrip(t, router, adminToken(t), "/users",
		map[string]any{"username": "pg-user", "email": "pg-user@example.com", "password": "S3cr3t!Pass", "roles": []string{"morador"}},
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

func TestHandler_Security_OtherCondominiumCannotAccessUsers(t *testing.T) {
	t.Run("memory", func(t *testing.T) { otherCondominiumCannotAccessUsers(t, newTestRouter(t)) })
	t.Run("postgres", func(t *testing.T) { otherCondominiumCannotAccessUsers(t, newPostgresRouter(t)) })
}

func otherCondominiumCannotAccessUsers(t *testing.T, router http.Handler) {
	t.Helper()
	tokA := adminToken(t)
	tokB := tokenFor(t, "admin-b", condoB, "sindico")

	created := createUser(t, router, tokA, map[string]any{
		"username": "ana", "email": "ana@condo-a.test", "password": strongPass,
	})
	id := fmt.Sprintf("%v", created["id"])

	if rec := serve(router, authRequest(http.MethodGet, "/users/"+id, tokB, nil)); rec.Code != http.StatusNotFound {
		t.Errorf("GET: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, authRequest(http.MethodPut, "/users/"+id, tokB, map[string]any{"username": "pwned", "roles": []string{"sindico"}})); rec.Code != http.StatusNotFound {
		t.Errorf("PUT: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, authRequest(http.MethodDelete, "/users/"+id, tokB, nil)); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE: expected 404, got %d", rec.Code)
	}
	listB := decodeList(t, serve(router, authRequest(http.MethodGet, "/users", tokB, nil)))
	if listB.Total != 0 || len(listB.Items) != 0 {
		t.Errorf("condo-b list must be empty, got %+v", listB)
	}
	// /users/me with a condo-b token carrying a condo-a user's ID must not resolve.
	if rec := serve(router, authRequest(http.MethodGet, "/users/me", tokenFor(t, id, condoB, "morador"), nil)); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /users/me cross-tenant: expected 401, got %d", rec.Code)
	}

	// The user is untouched in condo-a.
	rec := serve(router, authRequest(http.MethodGet, "/users/"+id, tokA, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("condo-a GET: expected 200, got %d", rec.Code)
	}
	var got map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&got)
	roles, _ := got["roles"].([]any)
	if got["username"] != "ana" || len(roles) != 1 || roles[0] != "morador" {
		t.Errorf("user changed by another condominium: %v", got)
	}
}

func TestIntegration_Users_EmailUniqueAcrossCondominiums(t *testing.T) {
	router := newPostgresRouter(t)
	createUser(t, router, adminToken(t), map[string]any{"username": "ana", "email": "Ana@X.io", "password": strongPass})

	rec := serve(router, authRequest(http.MethodPost, "/users", tokenFor(t, "admin-b", condoB, "sindico"), map[string]any{
		"username": "ana", "email": "ana@x.io", "password": strongPass,
	}))
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for same email in another condominium, got %d — %s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Users_MeAndPagination(t *testing.T) {
	router := newPostgresRouter(t)
	tok := adminToken(t)
	var firstID string
	for i := 1; i <= 3; i++ {
		u := createUser(t, router, tok, map[string]any{
			"username": fmt.Sprintf("pg%d", i), "email": fmt.Sprintf("pg%d@example.com", i), "password": strongPass,
		})
		if i == 1 {
			firstID = fmt.Sprintf("%v", u["id"])
		}
	}

	resp := decodeList(t, serve(router, authRequest(http.MethodGet, "/users?page=2&pageSize=2", tok, nil)))
	if resp.Page != 2 || resp.PageSize != 2 || resp.Total != 3 || len(resp.Items) != 1 || resp.Items[0]["username"] != "pg3" {
		t.Errorf("unexpected page: %+v", resp)
	}

	rec := serve(router, authRequest(http.MethodGet, "/users/me", tokenFor(t, firstID, condoA, "morador"), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/users/me: expected 200, got %d", rec.Code)
	}
	var me map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&me)
	if me["id"] != firstID || me["username"] != "pg1" {
		t.Errorf("unexpected /users/me: %v", me)
	}
	if _, ok := me["password"]; ok {
		t.Error("password must not be serialised")
	}
}

// crudRoundTrip drives create → get → list → update → delete through the HTTP
// handler backed by the PostgreSQL repository.
func crudRoundTrip(t *testing.T, router http.Handler, tok, base string, create, update map[string]any, check func(created, updated map[string]any)) {
	t.Helper()
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		return serve(router, authRequest(method, path, tok, body))
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
	list := decodeList(t, rec)
	if len(list.Items) != 1 || list.Total != 1 {
		t.Fatalf("list: expected 1 item (total 1), got %d (total %d)", len(list.Items), list.Total)
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
