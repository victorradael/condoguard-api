package resident_test

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
	"github.com/victorradael/condoguard/api/internal/resident"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"

const (
	condoA = dbtest.CondoA
	condoB = dbtest.CondoB
)

var handlerJWT = pkgjwt.MustNewService(handlerSecret)

func newRouter(repo resident.Repository) http.Handler {
	return resident.NewHandler(resident.NewService(repo), middleware.Authenticate(handlerJWT))
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	return newRouter(resident.NewInMemoryRepository())
}

func token(t *testing.T, userID, condoID, role string) string {
	t.Helper()
	tok, err := handlerJWT.GenerateToken(userID, condoID, []string{role})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// validToken is an admin of condo-a: the role allowed to manage units.
func validToken(t *testing.T) string { return token(t, "admin-1", condoA, authz.RoleSindico) }

// userToken is a morador (resident) of condo-a.
func userToken(t *testing.T, userID string) string {
	return token(t, userID, condoA, authz.RoleMorador)
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

func serve(router http.Handler, method, path, tok string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(method, path, tok, body))
	return rec
}

func createResident(t *testing.T, router http.Handler, tok string, unitNumber, condoID string) map[string]any {
	t.Helper()
	return createOwnedResident(t, router, tok, unitNumber, condoID, "user-1")
}

func createOwnedResident(t *testing.T, router http.Handler, tok, unitNumber, condoID, ownerID string) map[string]any {
	t.Helper()
	rec := serve(router, http.MethodPost, "/residents", tok, map[string]any{
		"unitNumber":    unitNumber,
		"floor":         1,
		"condominiumId": condoID,
		"ownerId":       ownerID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup createResident: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var r map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&r)
	return r
}

type listPage struct {
	Items    []map[string]any `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

func decodePage(t *testing.T, rec *httptest.ResponseRecorder) listPage {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var p listPage
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	return p
}

func idOf(r map[string]any) string { return fmt.Sprintf("%v", r["id"]) }

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestResidents_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodGet, "/residents", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GET /residents ────────────────────────────────────────────────────────────

func TestResidents_List_Empty_Returns200WithEmptyArray(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodGet, "/residents", validToken(t), nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if string(raw["items"]) != "[]" {
		t.Errorf("expected items to be [], got %s", raw["items"])
	}
	var p listPage
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Total != 0 || p.Page != 1 || p.PageSize != 20 {
		t.Errorf("expected empty page 1/20 total 0, got %+v", p)
	}
}

// ── POST /residents ───────────────────────────────────────────────────────────

func TestResidents_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/residents", validToken(t), map[string]any{
		"unitNumber":    "101",
		"floor":         1,
		"condominiumId": "condo-1",
		"ownerId":       "user-1",
	})

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var r map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&r)
	if r["id"] == nil || r["id"] == "" {
		t.Error("expected id in response")
	}
}

func TestResidents_Create_MissingUnitNumber_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/residents", validToken(t), map[string]any{
		"floor":         1,
		"condominiumId": "condo-1",
		"ownerId":       "user-1",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestResidents_Create_MissingCondominiumID_DefaultsToTenant(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/residents", validToken(t), map[string]any{
		"unitNumber": "101",
		"floor":      1,
		"ownerId":    "user-1",
	})

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 (condominiumId is optional), got %d", rec.Code)
	}
}

func TestResidents_Create_DuplicateUnitInCondominium_Returns409(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	createResident(t, router, tok, "101", "condo-1")

	rec := serve(router, http.MethodPost, "/residents", tok, map[string]any{
		"unitNumber":    "101",
		"floor":         2,
		"condominiumId": "condo-1",
		"ownerId":       "user-2",
	})

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestResidents_Create_SameUnitDifferentBlock_Returns201(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	serve(router, http.MethodPost, "/residents", tok, map[string]any{
		"unitNumber": "101", "floor": 1, "blockId": "6f1f8c3e-0000-4000-8000-000000000001",
	})

	rec := serve(router, http.MethodPost, "/units", tok, map[string]any{
		"unitNumber": "101", "floor": 1, "blockId": "6f1f8c3e-0000-4000-8000-000000000002",
	})

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 for same unit in another block, got %d", rec.Code)
	}
}

// ── GET /residents/{id} ───────────────────────────────────────────────────────

func TestResidents_GetByID_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	id := idOf(createResident(t, router, tok, "501", "condo-1"))

	rec := serve(router, http.MethodGet, "/residents/"+id, tok, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestResidents_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodGet, "/residents/ghost-id", validToken(t), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /residents/{id} ───────────────────────────────────────────────────────

func TestResidents_Update_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	id := idOf(createResident(t, router, tok, "601", "condo-1"))

	rec := serve(router, http.MethodPut, "/residents/"+id, tok, map[string]any{
		"unitNumber": "601",
		"floor":      9,
	})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["floor"] != float64(9) {
		t.Errorf("expected floor 9, got %v", updated["floor"])
	}
}

func TestResidents_Update_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPut, "/residents/ghost-id", validToken(t), map[string]any{
		"unitNumber": "101",
		"floor":      1,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── DELETE /residents/{id} ────────────────────────────────────────────────────

func TestResidents_Delete_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	id := idOf(createResident(t, router, tok, "701", "condo-1"))

	rec := serve(router, http.MethodDelete, "/residents/"+id, tok, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestResidents_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodDelete, "/residents/ghost-id", validToken(t), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Security: role-based authorization ────────────────────────────────────────

func TestHandler_Security_UserCannotCreateUnit_Returns403(t *testing.T) {
	router := newTestRouter(t)
	rec := serve(router, http.MethodPost, "/residents", userToken(t, "user-1"), map[string]any{
		"unitNumber": "101", "floor": 1, "condominiumId": "condo-1", "ownerId": "user-1",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d — %s", rec.Code, rec.Body.String())
	}
	if p := decodePage(t, serve(router, http.MethodGet, "/residents", validToken(t), nil)); p.Total != 0 {
		t.Errorf("forbidden create must not persist anything, got total %d", p.Total)
	}
}

func TestHandler_Security_UserCannotUpdateOwnUnit_Returns403(t *testing.T) {
	router := newTestRouter(t)
	id := idOf(createOwnedResident(t, router, validToken(t), "101", "condo-1", "user-1"))

	// Even the owner of the unit is not allowed to change it.
	rec := serve(router, http.MethodPut, "/residents/"+id, userToken(t, "user-1"), map[string]any{
		"unitNumber": "999", "floor": 9,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d — %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	_ = json.NewDecoder(serve(router, http.MethodGet, "/residents/"+id, validToken(t), nil).Body).Decode(&got)
	if got["unitNumber"] != "101" || got["floor"] != float64(1) {
		t.Errorf("forbidden update must not change the unit, got %v", got)
	}
}

func TestHandler_Security_UserCannotDeleteUnit_Returns403(t *testing.T) {
	router := newTestRouter(t)
	id := idOf(createOwnedResident(t, router, validToken(t), "101", "condo-1", "user-1"))

	rec := serve(router, http.MethodDelete, "/residents/"+id, userToken(t, "user-1"), nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d — %s", rec.Code, rec.Body.String())
	}
	if rec := serve(router, http.MethodGet, "/residents/"+id, validToken(t), nil); rec.Code != http.StatusOK {
		t.Errorf("forbidden delete must keep the unit, GET returned %d", rec.Code)
	}
}

func TestHandler_Security_UserListsOnlyOwnUnits(t *testing.T) {
	router := newTestRouter(t)
	admin := validToken(t)
	a1 := idOf(createOwnedResident(t, router, admin, "101", "condo-1", "user-a"))
	createOwnedResident(t, router, admin, "102", "condo-1", "user-b")
	a2 := idOf(createOwnedResident(t, router, admin, "103", "condo-1", "user-a"))

	p := decodePage(t, serve(router, http.MethodGet, "/residents", userToken(t, "user-a"), nil))
	if p.Total != 2 || len(p.Items) != 2 {
		t.Fatalf("expected 2 units for user-a, got total=%d items=%d", p.Total, len(p.Items))
	}
	for _, it := range p.Items {
		if it["ownerId"] != "user-a" {
			t.Errorf("user-a must only see own units, got %v", it)
		}
	}
	if idOf(p.Items[0]) != a1 || idOf(p.Items[1]) != a2 {
		t.Errorf("unexpected items: %v", p.Items)
	}

	// The admin still sees everything.
	if p := decodePage(t, serve(router, http.MethodGet, "/residents", admin, nil)); p.Total != 3 {
		t.Errorf("admin must see all 3 units, got %d", p.Total)
	}

	// A user without units gets an empty page.
	if p := decodePage(t, serve(router, http.MethodGet, "/residents", userToken(t, "user-c"), nil)); p.Total != 0 || len(p.Items) != 0 {
		t.Errorf("user-c must see nothing, got %+v", p)
	}
}

func TestHandler_Security_UserCannotGetOtherUsersUnit_Returns404(t *testing.T) {
	router := newTestRouter(t)
	admin := validToken(t)
	own := idOf(createOwnedResident(t, router, admin, "101", "condo-1", "user-a"))
	other := idOf(createOwnedResident(t, router, admin, "102", "condo-1", "user-b"))

	if rec := serve(router, http.MethodGet, "/residents/"+own, userToken(t, "user-a"), nil); rec.Code != http.StatusOK {
		t.Errorf("own unit: expected 200, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, "/residents/"+other, userToken(t, "user-a"), nil); rec.Code != http.StatusNotFound {
		t.Errorf("other user's unit: expected 404, got %d — %s", rec.Code, rec.Body.String())
	}
}

// ── Security: tenant isolation through the token's condominium ───────────────

func TestHandler_Security_OtherCondominiumTokenCannotAccessUnit_Returns404(t *testing.T) {
	router := newTestRouter(t)
	id := idOf(createOwnedResident(t, router, validToken(t), "101", "condo-1", "user-1"))

	// Admin of condo-b, and even a user with the same user ID as the owner.
	foreign := map[string]string{
		"admin of condo-b":         token(t, "admin-1", condoB, authz.RoleSindico),
		"same owner id in condo-b": token(t, "user-1", condoB, authz.RoleMorador),
	}
	for name, tok := range foreign {
		t.Run(name, func(t *testing.T) {
			if rec := serve(router, http.MethodGet, "/residents/"+id, tok, nil); rec.Code != http.StatusNotFound {
				t.Errorf("GET: expected 404, got %d", rec.Code)
			}
			if p := decodePage(t, serve(router, http.MethodGet, "/residents", tok, nil)); p.Total != 0 || len(p.Items) != 0 {
				t.Errorf("list: expected nothing from condo-a, got %+v", p)
			}
		})
	}

	adminB := foreign["admin of condo-b"]
	if rec := serve(router, http.MethodPut, "/residents/"+id, adminB, map[string]any{"unitNumber": "999", "floor": 9}); rec.Code != http.StatusNotFound {
		t.Errorf("PUT: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/residents/"+id, adminB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE: expected 404, got %d", rec.Code)
	}

	// The unit is untouched in condo-a.
	rec := serve(router, http.MethodGet, "/residents/"+id, validToken(t), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("condo-a GET: expected 200, got %d", rec.Code)
	}
	var got map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got["unitNumber"] != "101" || got["floor"] != float64(1) {
		t.Errorf("unit changed by another condominium: %v", got)
	}
}

func TestHandler_Security_CreateIsScopedToTokenCondominium(t *testing.T) {
	router := newTestRouter(t)
	adminB := token(t, "admin-b", condoB, authz.RoleSindico)

	// The body's condominiumId does not pick the tenant: the token does.
	createOwnedResident(t, router, adminB, "101", condoA, "user-1")

	if p := decodePage(t, serve(router, http.MethodGet, "/residents", validToken(t), nil)); p.Total != 0 {
		t.Errorf("condo-a must not see a unit created by condo-b, got %d", p.Total)
	}
	if p := decodePage(t, serve(router, http.MethodGet, "/residents", adminB, nil)); p.Total != 1 {
		t.Errorf("condo-b must see its own unit, got %d", p.Total)
	}
}

// ── Pagination ───────────────────────────────────────────────────────────────

func TestHandler_Pagination_PageAndPageSizeRespected(t *testing.T) {
	router := newTestRouter(t)
	admin := validToken(t)
	ids := make([]string, 0, 5)
	for i := range 5 {
		ids = append(ids, idOf(createResident(t, router, admin, fmt.Sprintf("%d", 100+i), "condo-1")))
	}

	cases := []struct {
		query   string
		page    int
		size    int
		wantIDs []string
	}{
		{"?page=1&pageSize=2", 1, 2, ids[0:2]},
		{"?page=2&pageSize=2", 2, 2, ids[2:4]},
		{"?page=3&pageSize=2", 3, 2, ids[4:5]},
		{"?page=4&pageSize=2", 4, 2, nil},
		{"?pageSize=100", 1, 100, ids},
		{"", 1, 20, ids},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			p := decodePage(t, serve(router, http.MethodGet, "/residents"+c.query, admin, nil))
			if p.Page != c.page || p.PageSize != c.size || p.Total != 5 {
				t.Errorf("expected page=%d pageSize=%d total=5, got %+v", c.page, c.size, p)
			}
			if len(p.Items) != len(c.wantIDs) {
				t.Fatalf("expected %d items, got %d", len(c.wantIDs), len(p.Items))
			}
			for i, it := range p.Items {
				if idOf(it) != c.wantIDs[i] {
					t.Errorf("item %d: expected %s, got %s", i, c.wantIDs[i], idOf(it))
				}
			}
		})
	}
}

func TestHandler_Pagination_UserTotalCountsOnlyOwnUnits(t *testing.T) {
	router := newTestRouter(t)
	admin := validToken(t)
	for i := range 3 {
		createOwnedResident(t, router, admin, fmt.Sprintf("a%d", i), "condo-1", "user-a")
		createOwnedResident(t, router, admin, fmt.Sprintf("b%d", i), "condo-1", "user-b")
	}

	p := decodePage(t, serve(router, http.MethodGet, "/residents?page=2&pageSize=2", userToken(t, "user-a"), nil))
	if p.Total != 3 || len(p.Items) != 1 || p.Items[0]["ownerId"] != "user-a" {
		t.Errorf("expected last page of user-a's 3 units, got %+v", p)
	}
}

func TestHandler_Pagination_InvalidParams_Returns400(t *testing.T) {
	router := newTestRouter(t)
	for _, q := range []string{
		"?page=0", "?page=-1", "?page=abc", "?page=1000001",
		"?pageSize=0", "?pageSize=101", "?pageSize=x", "?page=1&pageSize=-5",
	} {
		t.Run(q, func(t *testing.T) {
			if rec := serve(router, http.MethodGet, "/residents"+q, validToken(t), nil); rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rec.Code)
			}
		})
	}
}

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func TestIntegration_Residents_CRUD(t *testing.T) {
	router := newRouter(resident.NewPostgresRepository(dbtest.NewPool(t, dbtest.CondoA, dbtest.CondoB)))

	crudRoundTrip(t, router, validToken(t), "/residents",
		map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": "condo-1", "ownerId": "user-1"},
		map[string]any{"unitNumber": "102", "floor": 2},
		func(_, updated map[string]any) {
			if updated["unitNumber"] != "102" || updated["floor"] != float64(2) {
				t.Errorf("update not persisted: %v", updated)
			}
			if updated["condominiumId"] != "condo-1" {
				t.Errorf("condominiumId must round-trip, got %v", updated["condominiumId"])
			}
		})
}

// Regression (FAB-11 N1): a floor outside int32 is a validation error, not a 500.
func TestIntegration_Residents_Update_FloorOutOfRange_Returns422(t *testing.T) {
	router := newRouter(resident.NewPostgresRepository(dbtest.NewPool(t, dbtest.CondoA, dbtest.CondoB)))
	tok := validToken(t)
	id := idOf(createResident(t, router, tok, "901", "condo-1"))

	rec := serve(router, http.MethodPut, "/residents/"+id, tok, map[string]any{
		"unitNumber": "901",
		"floor":      int64(1) << 40,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d — %s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Security_Residents_RoleAndTenantIsolation(t *testing.T) {
	router := newRouter(resident.NewPostgresRepository(dbtest.NewPool(t, dbtest.CondoA, dbtest.CondoB)))
	admin := validToken(t)
	own := idOf(createOwnedResident(t, router, admin, "101", "condo-1", "user-a"))
	other := idOf(createOwnedResident(t, router, admin, "102", "condo-1", "user-b"))
	userA := userToken(t, "user-a")
	adminB := token(t, "admin-1", condoB, authz.RoleSindico)

	p := decodePage(t, serve(router, http.MethodGet, "/residents", userA, nil))
	if p.Total != 1 || len(p.Items) != 1 || idOf(p.Items[0]) != own {
		t.Errorf("user-a list: expected only own unit, got %+v", p)
	}
	if rec := serve(router, http.MethodGet, "/residents/"+other, userA, nil); rec.Code != http.StatusNotFound {
		t.Errorf("user-a GET other: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/residents/"+own, userA, nil); rec.Code != http.StatusForbidden {
		t.Errorf("user-a DELETE: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, "/residents/"+own, adminB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b GET: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, "/residents/"+own, adminB, map[string]any{"floor": 7}); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b PUT: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/residents/"+own, adminB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b DELETE: expected 404, got %d", rec.Code)
	}
	if p := decodePage(t, serve(router, http.MethodGet, "/residents", admin, nil)); p.Total != 2 {
		t.Errorf("condo-a admin: expected 2 units intact, got %d", p.Total)
	}
}

// crudRoundTrip drives create → get → list → update → delete through the HTTP
// handler backed by the PostgreSQL repository.
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
	list := decodePage(t, do(http.MethodGet, base, nil))
	if len(list.Items) != 1 || list.Total != 1 {
		t.Fatalf("list: expected 1 item, got %d (total %d)", len(list.Items), list.Total)
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
