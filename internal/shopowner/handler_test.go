package shopowner_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/shopowner"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"

const (
	condoA = "condo-a"
	condoB = "condo-b"
)

var testJWT = pkgjwt.MustNewService(handlerSecret)

func newRouter(repo shopowner.Repository) http.Handler {
	return shopowner.NewHandler(shopowner.NewService(repo), middleware.Authenticate(testJWT))
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	return newRouter(shopowner.NewInMemoryRepository())
}

func tokenFor(t *testing.T, userID, condo string, roles ...string) string {
	t.Helper()
	tok, err := testJWT.GenerateToken(userID, condo, roles)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// adminToken is an admin of condo-a: may manage every shop of the condominium.
func adminToken(t *testing.T) string {
	return tokenFor(t, "admin-1", condoA, authz.RoleAdmin)
}

// userToken is a plain ROLE_USER of condo-a.
func userToken(t *testing.T, userID string) string {
	return tokenFor(t, userID, condoA, authz.RoleUser)
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

func serve(router http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(method, path, token, body))
	return rec
}

type listBody struct {
	Items    []map[string]any `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) listBody {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var l listBody
	if err := json.NewDecoder(rec.Body).Decode(&l); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return l
}

func createShopFor(t *testing.T, router http.Handler, tok, shopName, cnpj, ownerID string) map[string]any {
	t.Helper()
	rec := serve(router, http.MethodPost, "/shop-owners", tok, map[string]any{
		"shopName": shopName,
		"cnpj":     cnpj,
		"floor":    1,
		"ownerId":  ownerID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup createShop: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var s map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&s)
	return s
}

func createShop(t *testing.T, router http.Handler, tok, shopName, cnpj string) map[string]any {
	t.Helper()
	return createShopFor(t, router, tok, shopName, cnpj, "user-1")
}

// Valid CNPJs used across the tests.
var shopCNPJs = []string{
	"11.222.333/0001-81",
	"45.997.418/0001-53",
	"11.444.777/0001-61",
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestShopOwners_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	for _, p := range shopowner.Prefixes {
		if rec := serve(router, http.MethodGet, p, "", nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", p, rec.Code)
		}
	}
}

// ── GET /shop-owners ──────────────────────────────────────────────────────────

func TestShopOwners_List_Empty_Returns200WithEmptyPage(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodGet, "/shop-owners", adminToken(t), nil)

	if !bytes.Contains(rec.Body.Bytes(), []byte(`"items":[]`)) {
		t.Errorf("expected items to be [] (not null), got %s", rec.Body.String())
	}
	l := decodeList(t, rec)
	if len(l.Items) != 0 || l.Total != 0 || l.Page != 1 || l.PageSize != 20 {
		t.Errorf("expected empty page 1/20, got %+v", l)
	}
}

// ── POST /shop-owners ─────────────────────────────────────────────────────────

func TestShopOwners_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/shop-owners", adminToken(t), map[string]any{
		"shopName": "Papelaria",
		"cnpj":     "11.222.333/0001-81",
		"floor":    2,
		"ownerId":  "user-1",
	})

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

func TestShopOwners_Create_InvalidJSON_Returns400(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/shop-owners", bytes.NewReader([]byte("{")))
	req.Header.Set("Authorization", "Bearer "+adminToken(t))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestShopOwners_Create_InvalidCNPJ_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/shop-owners", adminToken(t), map[string]any{
		"shopName": "Loja Inválida",
		"cnpj":     "00000000000000",
		"floor":    1,
		"ownerId":  "user-1",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestShopOwners_Create_MissingShopName_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/shop-owners", adminToken(t), map[string]any{
		"cnpj":    "11.222.333/0001-81",
		"floor":   1,
		"ownerId": "user-1",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestShopOwners_Create_DuplicateCNPJ_Returns409(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)

	createShop(t, router, tok, "Loja A", "11.222.333/0001-81")

	rec := serve(router, http.MethodPost, "/shop-owners", tok, map[string]any{
		"shopName": "Loja B",
		"cnpj":     "11222333000181", // same CNPJ raw
		"floor":    3,
		"ownerId":  "user-2",
	})

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

// ── GET /shop-owners/{id} ─────────────────────────────────────────────────────

func TestShopOwners_GetByID_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	s := createShop(t, router, tok, "Açougue", "11.222.333/0001-81")
	id := fmt.Sprintf("%v", s["id"])

	if rec := serve(router, http.MethodGet, "/shop-owners/"+id, tok, nil); rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestShopOwners_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	if rec := serve(router, http.MethodGet, "/shop-owners/ghost-id", adminToken(t), nil); rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /shop-owners/{id} ─────────────────────────────────────────────────────

func TestShopOwners_Update_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	s := createShop(t, router, tok, "Livraria Velha", "11.222.333/0001-81")
	id := fmt.Sprintf("%v", s["id"])

	rec := serve(router, http.MethodPut, "/shop-owners/"+id, tok, map[string]any{
		"shopName": "Livraria Nova",
		"floor":    5,
	})

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
	rec := serve(router, http.MethodPut, "/shop-owners/ghost-id", adminToken(t), map[string]any{"shopName": "X"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── DELETE /shop-owners/{id} ──────────────────────────────────────────────────

func TestShopOwners_Delete_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	s := createShop(t, router, tok, "Floricultura", "11.222.333/0001-81")
	id := fmt.Sprintf("%v", s["id"])

	if rec := serve(router, http.MethodDelete, "/shop-owners/"+id, tok, nil); rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, "/shop-owners/"+id, tok, nil); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete: expected 404, got %d", rec.Code)
	}
}

func TestShopOwners_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	if rec := serve(router, http.MethodDelete, "/shop-owners/ghost-id", adminToken(t), nil); rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Routes: canonical /shop-owners and the /shopOwners alias ──────────────────

func TestHandler_BothPrefixes_ServeFullCRUD(t *testing.T) {
	if len(shopowner.Prefixes) != 2 || shopowner.Prefixes[0] != "/shop-owners" || shopowner.Prefixes[1] != "/shopOwners" {
		t.Fatalf("unexpected prefixes: %v", shopowner.Prefixes)
	}
	for i, prefix := range shopowner.Prefixes {
		t.Run(prefix, func(t *testing.T) {
			router := newTestRouter(t)
			crudRoundTrip(t, router, adminToken(t), prefix,
				map[string]any{"shopName": "Loja", "cnpj": shopCNPJs[i], "floor": 1, "ownerId": "user-1"},
				map[string]any{"shopName": "Loja Nova"},
				func(_, updated map[string]any) {
					if updated["shopName"] != "Loja Nova" {
						t.Errorf("update not applied: %v", updated)
					}
				})
		})
	}
}

func TestHandler_BothPrefixes_ShareTheSameData(t *testing.T) {
	router := newTestRouter(t)
	tok := adminToken(t)
	id := fmt.Sprintf("%v", createShop(t, router, tok, "Loja", shopCNPJs[0])["id"])

	if rec := serve(router, http.MethodGet, "/shopOwners/"+id, tok, nil); rec.Code != http.StatusOK {
		t.Errorf("alias GET of a shop created via canonical path: expected 200, got %d", rec.Code)
	}
	if l := decodeList(t, serve(router, http.MethodGet, "/shopOwners", tok, nil)); l.Total != 1 {
		t.Errorf("alias list: expected total 1, got %d", l.Total)
	}
}

// ── Security: ROLE_USER cannot write ──────────────────────────────────────────

func TestHandler_Security_UserCannotCreate_Returns403(t *testing.T) {
	router := newTestRouter(t)
	for _, prefix := range shopowner.Prefixes {
		rec := serve(router, http.MethodPost, prefix, userToken(t, "user-1"), map[string]any{
			"shopName": "Loja Pirata",
			"cnpj":     shopCNPJs[0],
			"floor":    1,
			"ownerId":  "user-1", // even claiming the shop for themselves
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403, got %d — %s", prefix, rec.Code, rec.Body.String())
		}
	}
	if l := decodeList(t, serve(router, http.MethodGet, "/shop-owners", adminToken(t), nil)); l.Total != 0 {
		t.Errorf("forbidden create persisted a shop: total %d", l.Total)
	}
}

func TestHandler_Security_UserCannotUpdateOrDelete_Returns403(t *testing.T) {
	router := newTestRouter(t)
	admin := adminToken(t)
	// The shop belongs to user-1, and even its owner may not change it.
	id := fmt.Sprintf("%v", createShopFor(t, router, admin, "Original", shopCNPJs[0], "user-1")["id"])

	for _, tok := range []string{userToken(t, "user-1"), userToken(t, "user-2")} {
		for _, prefix := range shopowner.Prefixes {
			rec := serve(router, http.MethodPut, prefix+"/"+id, tok, map[string]any{"shopName": "Hacked", "floor": 9})
			if rec.Code != http.StatusForbidden {
				t.Errorf("PUT %s: expected 403, got %d", prefix, rec.Code)
			}
			if rec := serve(router, http.MethodDelete, prefix+"/"+id, tok, nil); rec.Code != http.StatusForbidden {
				t.Errorf("DELETE %s: expected 403, got %d", prefix, rec.Code)
			}
		}
	}

	rec := serve(router, http.MethodGet, "/shop-owners/"+id, admin, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("shop was deleted by a forbidden request: %d", rec.Code)
	}
	var s map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&s)
	if s["shopName"] != "Original" || s["floor"] != float64(1) {
		t.Errorf("shop changed by a forbidden request: %v", s)
	}
}

// ── Security: ROLE_USER only sees their own shops ─────────────────────────────

func TestHandler_Security_UserListsOnlyOwnShops(t *testing.T) {
	router := newTestRouter(t)
	admin := adminToken(t)
	a1 := fmt.Sprintf("%v", createShopFor(t, router, admin, "A1", shopCNPJs[0], "user-a")["id"])
	a2 := fmt.Sprintf("%v", createShopFor(t, router, admin, "A2", shopCNPJs[1], "user-a")["id"])
	createShopFor(t, router, admin, "B1", shopCNPJs[2], "user-b")

	for _, prefix := range shopowner.Prefixes {
		l := decodeList(t, serve(router, http.MethodGet, prefix, userToken(t, "user-a"), nil))
		if l.Total != 2 || len(l.Items) != 2 {
			t.Fatalf("%s: user-a expected 2 shops, got total %d items %d", prefix, l.Total, len(l.Items))
		}
		for _, it := range l.Items {
			if it["ownerId"] != "user-a" {
				t.Errorf("%s: user-a sees a shop of %v", prefix, it["ownerId"])
			}
		}
		if l.Items[0]["id"] != a1 || l.Items[1]["id"] != a2 {
			t.Errorf("%s: unexpected items/order: %v", prefix, l.Items)
		}
	}

	if l := decodeList(t, serve(router, http.MethodGet, "/shop-owners", userToken(t, "user-c"), nil)); l.Total != 0 || len(l.Items) != 0 {
		t.Errorf("user without shops: expected empty, got %+v", l)
	}
	if l := decodeList(t, serve(router, http.MethodGet, "/shop-owners", admin, nil)); l.Total != 3 {
		t.Errorf("admin: expected total 3, got %d", l.Total)
	}
}

func TestHandler_Security_UserCannotGetAnotherUsersShop_Returns404(t *testing.T) {
	router := newTestRouter(t)
	admin := adminToken(t)
	own := fmt.Sprintf("%v", createShopFor(t, router, admin, "A", shopCNPJs[0], "user-a")["id"])
	other := fmt.Sprintf("%v", createShopFor(t, router, admin, "B", shopCNPJs[1], "user-b")["id"])

	for _, prefix := range shopowner.Prefixes {
		if rec := serve(router, http.MethodGet, prefix+"/"+own, userToken(t, "user-a"), nil); rec.Code != http.StatusOK {
			t.Errorf("%s: own shop expected 200, got %d", prefix, rec.Code)
		}
		rec := serve(router, http.MethodGet, prefix+"/"+other, userToken(t, "user-a"), nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: another user's shop expected 404, got %d", prefix, rec.Code)
		}
		if bytes.Contains(rec.Body.Bytes(), []byte(shopCNPJs[1])) {
			t.Errorf("%s: 404 body leaks the shop: %s", prefix, rec.Body.String())
		}
	}
}

// ── Security: another condominium's token sees nothing ────────────────────────

func TestHandler_Security_OtherCondominiumAdmin_CannotTouchShop_Returns404(t *testing.T) {
	router := newTestRouter(t)
	adminA := adminToken(t)
	id := fmt.Sprintf("%v", createShopFor(t, router, adminA, "Loja A", shopCNPJs[0], "user-1")["id"])

	adminB := tokenFor(t, "admin-b", condoB, authz.RoleAdmin)
	// Same user ID as the shop owner but in another condominium.
	ownerInB := tokenFor(t, "user-1", condoB, authz.RoleUser)

	for _, prefix := range shopowner.Prefixes {
		for name, tok := range map[string]string{"adminB": adminB, "ownerInB": ownerInB} {
			if rec := serve(router, http.MethodGet, prefix+"/"+id, tok, nil); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s GET: expected 404, got %d", prefix, name, rec.Code)
			}
			if l := decodeList(t, serve(router, http.MethodGet, prefix, tok, nil)); l.Total != 0 || len(l.Items) != 0 {
				t.Errorf("%s %s list: expected empty, got %+v", prefix, name, l)
			}
		}
		rec := serve(router, http.MethodPut, prefix+"/"+id, adminB, map[string]any{"shopName": "Hacked", "floor": 9})
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s adminB PUT: expected 404, got %d", prefix, rec.Code)
		}
		if rec := serve(router, http.MethodDelete, prefix+"/"+id, adminB, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s adminB DELETE: expected 404, got %d", prefix, rec.Code)
		}
	}

	rec := serve(router, http.MethodGet, "/shop-owners/"+id, adminA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("shop of condo-a gone after condo-b requests: %d", rec.Code)
	}
	var s map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&s)
	if s["shopName"] != "Loja A" || s["floor"] != float64(1) {
		t.Errorf("shop of condo-a changed by condo-b: %v", s)
	}
}

func TestHandler_Security_SameCNPJInOtherCondominium_IsIndependent(t *testing.T) {
	router := newTestRouter(t)
	createShop(t, router, adminToken(t), "Loja A", shopCNPJs[0])
	adminB := tokenFor(t, "admin-b", condoB, authz.RoleAdmin)
	createShop(t, router, adminB, "Loja B", shopCNPJs[0]) // fails the test on 409

	if l := decodeList(t, serve(router, http.MethodGet, "/shop-owners", adminB, nil)); l.Total != 1 || l.Items[0]["shopName"] != "Loja B" {
		t.Errorf("condo-b list: %+v", l)
	}
}

// ── Pagination ────────────────────────────────────────────────────────────────

func TestHandler_List_Pagination(t *testing.T) {
	router := newTestRouter(t)
	admin := adminToken(t)
	ids := make([]string, 0, len(shopCNPJs))
	for i, c := range shopCNPJs {
		ids = append(ids, fmt.Sprintf("%v", createShopFor(t, router, admin, fmt.Sprintf("Loja %d", i), c, "user-1")["id"]))
	}

	cases := []struct {
		query     string
		page, siz int
		want      []string
	}{
		{"?page=1&pageSize=2", 1, 2, ids[:2]},
		{"?page=2&pageSize=2", 2, 2, ids[2:]},
		{"?page=3&pageSize=2", 3, 2, nil},
		{"?pageSize=1", 1, 1, ids[:1]},
		{"?page=1&pageSize=100", 1, 100, ids},
		{"", 1, 20, ids},
	}
	for _, prefix := range shopowner.Prefixes {
		for _, tc := range cases {
			l := decodeList(t, serve(router, http.MethodGet, prefix+tc.query, admin, nil))
			if l.Page != tc.page || l.PageSize != tc.siz || l.Total != int64(len(ids)) {
				t.Errorf("%s%s: got page=%d pageSize=%d total=%d", prefix, tc.query, l.Page, l.PageSize, l.Total)
			}
			if len(l.Items) != len(tc.want) {
				t.Errorf("%s%s: expected %d items, got %d", prefix, tc.query, len(tc.want), len(l.Items))
				continue
			}
			for i, id := range tc.want {
				if l.Items[i]["id"] != id {
					t.Errorf("%s%s: item %d = %v, want %s", prefix, tc.query, i, l.Items[i]["id"], id)
				}
			}
		}
	}
}

func TestHandler_List_Pagination_TotalCountsOnlyCallersShops(t *testing.T) {
	router := newTestRouter(t)
	admin := adminToken(t)
	createShopFor(t, router, admin, "A1", shopCNPJs[0], "user-a")
	createShopFor(t, router, admin, "B1", shopCNPJs[1], "user-b")
	createShopFor(t, router, admin, "A2", shopCNPJs[2], "user-a")

	l := decodeList(t, serve(router, http.MethodGet, "/shop-owners?page=2&pageSize=1", userToken(t, "user-a"), nil))
	if l.Total != 2 || len(l.Items) != 1 || l.Items[0]["shopName"] != "A2" {
		t.Errorf("user-a page 2: %+v", l)
	}
}

func TestHandler_List_InvalidPagination_Returns400(t *testing.T) {
	router := newTestRouter(t)
	for _, prefix := range shopowner.Prefixes {
		for _, q := range []string{
			"?page=0", "?page=-1", "?page=abc", "?page=1.5", "?page=1000001",
			"?pageSize=0", "?pageSize=101", "?pageSize=x", "?pageSize=-5",
		} {
			if rec := serve(router, http.MethodGet, prefix+q, adminToken(t), nil); rec.Code != http.StatusBadRequest {
				t.Errorf("%s%s: expected 400, got %d", prefix, q, rec.Code)
			}
		}
	}
}

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func newPostgresRouter(t *testing.T) http.Handler {
	t.Helper()
	return newRouter(shopowner.NewPostgresRepository(dbtest.NewPool(t)))
}

func TestIntegration_ShopOwners_CRUD(t *testing.T) {
	router := newPostgresRouter(t)

	rawCNPJs := []string{"11222333000181", "45997418000153"}
	for i, prefix := range shopowner.Prefixes {
		crudRoundTrip(t, router, adminToken(t), prefix,
			map[string]any{"shopName": "Padaria", "cnpj": rawCNPJs[i], "floor": 0, "ownerId": "user-1"},
			map[string]any{"shopName": "Padaria Nova", "floor": 1},
			func(_, updated map[string]any) {
				if updated["shopName"] != "Padaria Nova" {
					t.Errorf("update not persisted: %v", updated)
				}
				if updated["cnpj"] != shopCNPJs[i] {
					t.Errorf("expected formatted CNPJ, got %v", updated["cnpj"])
				}
			})
	}
}

func TestIntegration_Security_ShopOwners_RolesAndTenants(t *testing.T) {
	router := newPostgresRouter(t)
	admin := adminToken(t)
	a := fmt.Sprintf("%v", createShopFor(t, router, admin, "A", shopCNPJs[0], "user-a")["id"])
	b := fmt.Sprintf("%v", createShopFor(t, router, admin, "B", shopCNPJs[1], "user-b")["id"])
	createShopFor(t, router, admin, "A2", shopCNPJs[2], "user-a")

	ua := userToken(t, "user-a")
	if rec := serve(router, http.MethodPost, "/shop-owners", ua, map[string]any{"shopName": "X", "cnpj": "11444777000161", "ownerId": "user-a"}); rec.Code != http.StatusForbidden {
		t.Errorf("user POST: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, "/shop-owners/"+a, ua, map[string]any{"shopName": "X"}); rec.Code != http.StatusForbidden {
		t.Errorf("user PUT: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/shop-owners/"+a, ua, nil); rec.Code != http.StatusForbidden {
		t.Errorf("user DELETE: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, "/shop-owners/"+b, ua, nil); rec.Code != http.StatusNotFound {
		t.Errorf("user GET of another user's shop: expected 404, got %d", rec.Code)
	}
	l := decodeList(t, serve(router, http.MethodGet, "/shop-owners?pageSize=1", ua, nil))
	if l.Total != 2 || len(l.Items) != 1 || l.Items[0]["id"] != a {
		t.Errorf("user list page 1/1: %+v", l)
	}

	adminB := tokenFor(t, "admin-b", condoB, authz.RoleAdmin)
	if rec := serve(router, http.MethodGet, "/shop-owners/"+a, adminB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b GET: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, "/shop-owners/"+a, adminB, map[string]any{"shopName": "X"}); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b PUT: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/shop-owners/"+a, adminB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b DELETE: expected 404, got %d", rec.Code)
	}
	if l := decodeList(t, serve(router, http.MethodGet, "/shop-owners", adminB, nil)); l.Total != 0 {
		t.Errorf("condo-b list: expected empty, got %+v", l)
	}
	if l := decodeList(t, serve(router, http.MethodGet, "/shop-owners", admin, nil)); l.Total != 3 {
		t.Errorf("condo-a admin list after attacks: expected 3, got %d", l.Total)
	}
}

// Regression (FAB-11 N1): a floor outside int32 is a validation error, not a 500.
func TestIntegration_ShopOwners_Update_FloorOutOfRange_Returns422(t *testing.T) {
	router := newPostgresRouter(t)
	tok := adminToken(t)
	id := fmt.Sprintf("%v", createShop(t, router, tok, "Banca", "11222333000181")["id"])

	rec := serve(router, http.MethodPut, "/shop-owners/"+id, tok, map[string]any{
		"shopName": "Banca",
		"floor":    int64(1) << 40,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d — %s", rec.Code, rec.Body.String())
	}
}

// crudRoundTrip drives create → get → list → update → delete through the HTTP handler.
func crudRoundTrip(t *testing.T, router http.Handler, tok, base string, create, update map[string]any, check func(created, updated map[string]any)) {
	t.Helper()
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		return serve(router, method, path, tok, body)
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
	list := decodeList(t, do(http.MethodGet, base, nil))
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0]["id"] != id {
		t.Fatalf("list: expected exactly the created shop, got %+v", list)
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
