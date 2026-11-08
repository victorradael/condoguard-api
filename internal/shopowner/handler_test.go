package shopowner_test

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
	"github.com/victorradael/condoguard/api/internal/shopowner"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	repo := shopowner.NewInMemoryRepository()
	svc := shopowner.NewService(repo)
	jwtSvc := pkgjwt.NewService(handlerSecret)
	return shopowner.NewHandler(svc, middleware.Authenticate(jwtSvc))
}

func validToken(t *testing.T) string {
	t.Helper()
	svc := pkgjwt.NewService(handlerSecret)
	tok, err := svc.GenerateToken("user-1", []string{"ROLE_USER"})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func authReq(method, path, token string, body any) *http.Request {
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

func createShop(t *testing.T, router http.Handler, tok, shopName, cnpj string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/shopOwners", tok, map[string]any{
		"shopName": shopName,
		"cnpj":     cnpj,
		"floor":    1,
		"ownerId":  "user-1",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup createShop: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var s map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&s)
	return s
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestShopOwners_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/shopOwners", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GET /shopOwners ───────────────────────────────────────────────────────────

func TestShopOwners_List_Empty_Returns200WithEmptyArray(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/shopOwners", validToken(t), nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var list []any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected empty array, got %v", list)
	}
}

// ── POST /shopOwners ──────────────────────────────────────────────────────────

func TestShopOwners_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/shopOwners", validToken(t), map[string]any{
		"shopName": "Papelaria",
		"cnpj":     "11.222.333/0001-81",
		"floor":    2,
		"ownerId":  "user-1",
	}))

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var s map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&s)
	if s["id"] == nil || s["id"] == "" {
		t.Error("expected id in response")
	}
	// CNPJ deve estar formatado na resposta
	if s["cnpj"] != "11.222.333/0001-81" {
		t.Errorf("expected formatted CNPJ in response, got %v", s["cnpj"])
	}
}

func TestShopOwners_Create_InvalidCNPJ_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/shopOwners", validToken(t), map[string]any{
		"shopName": "Loja Inválida",
		"cnpj":     "00000000000000",
		"floor":    1,
		"ownerId":  "user-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestShopOwners_Create_MissingShopName_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/shopOwners", validToken(t), map[string]any{
		"cnpj":    "11.222.333/0001-81",
		"floor":   1,
		"ownerId": "user-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestShopOwners_Create_DuplicateCNPJ_Returns409(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	createShop(t, router, tok, "Loja A", "11.222.333/0001-81")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/shopOwners", tok, map[string]any{
		"shopName": "Loja B",
		"cnpj":     "11222333000181", // same CNPJ raw
		"floor":    3,
		"ownerId":  "user-2",
	}))

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

// ── GET /shopOwners/{id} ──────────────────────────────────────────────────────

func TestShopOwners_GetByID_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	s := createShop(t, router, tok, "Açougue", "11.222.333/0001-81")
	id := fmt.Sprintf("%v", s["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/shopOwners/"+id, tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestShopOwners_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/shopOwners/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /shopOwners/{id} ──────────────────────────────────────────────────────

func TestShopOwners_Update_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	s := createShop(t, router, tok, "Livraria Velha", "11.222.333/0001-81")
	id := fmt.Sprintf("%v", s["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/shopOwners/"+id, tok, map[string]any{
		"shopName": "Livraria Nova",
		"floor":    5,
	}))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["shopName"] != "Livraria Nova" {
		t.Errorf("expected shopName 'Livraria Nova', got %v", updated["shopName"])
	}
	if updated["floor"] != float64(5) {
		t.Errorf("expected floor 5, got %v", updated["floor"])
	}
}

func TestShopOwners_Update_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/shopOwners/ghost-id", validToken(t), map[string]any{
		"shopName": "X",
	}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── DELETE /shopOwners/{id} ───────────────────────────────────────────────────

func TestShopOwners_Delete_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	s := createShop(t, router, tok, "Floricultura", "11.222.333/0001-81")
	id := fmt.Sprintf("%v", s["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/shopOwners/"+id, tok, nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestShopOwners_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/shopOwners/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func TestIntegration_ShopOwners_CRUD(t *testing.T) {
	pool := dbtest.NewPool(t)
	router := tenant.Middleware("condo-test")(shopowner.NewHandler(shopowner.NewService(shopowner.NewPostgresRepository(pool)), middleware.Authenticate(pkgjwt.NewService(handlerSecret))))

	crudRoundTrip(t, router, validToken(t), "/shopOwners",
		map[string]any{"shopName": "Padaria", "cnpj": "11222333000181", "floor": 0, "ownerId": "user-1"},
		map[string]any{"shopName": "Padaria Nova", "floor": 1},
		func(_, updated map[string]any) {
			if updated["shopName"] != "Padaria Nova" {
				t.Errorf("update not persisted: %v", updated)
			}
			if updated["cnpj"] != "11.222.333/0001-81" {
				t.Errorf("expected formatted CNPJ, got %v", updated["cnpj"])
			}
		})
}

// crudRoundTrip drives create → get → list → update → delete through the HTTP
// handler backed by the PostgreSQL repository.
func crudRoundTrip(t *testing.T, router http.Handler, tok, base string, create, update map[string]any, check func(created, updated map[string]any)) {
	t.Helper()
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, authReq(method, path, tok, body))
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
