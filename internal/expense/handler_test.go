package expense_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/middleware"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"

var handlerJWT = pkgjwt.MustNewService(handlerSecret)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	svc := expense.NewService(expense.NewInMemoryRepository(), testUnits)
	return expense.NewHandler(svc, middleware.Authenticate(handlerJWT))
}

// tokenFor issues a token for userID in condoID; the token's condominium is the tenant.
func tokenFor(t *testing.T, userID, condoID string, roles ...string) string {
	t.Helper()
	tok, err := handlerJWT.GenerateToken(userID, condoID, roles)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// validToken is an admin of condo-a (expense management is admin-only).
func validToken(t *testing.T) string {
	t.Helper()
	return tokenFor(t, "admin-1", "condo-a", authz.RoleAdmin)
}

type listBody struct {
	Items    []map[string]any `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) listBody {
	t.Helper()
	var b listBody
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatalf("decode list: %v — %s", err, rec.Body.String())
	}
	return b
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

func createExpense(t *testing.T, router http.Handler, tok string, extra map[string]any) map[string]any {
	t.Helper()
	payload := map[string]any{
		"description": "Taxa Condomínio",
		"amountCents": 15000,
		"dueDate":     "2026-03-31T00:00:00Z",
		"residentId":  "resident-1",
	}
	for k, v := range extra {
		payload[k] = v
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/expenses", tok, payload))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup createExpense: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var e map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&e)
	return e
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestExpenses_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/expenses", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GET /expenses ─────────────────────────────────────────────────────────────

func TestExpenses_List_Empty_Returns200WithEmptyArray(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/expenses", validToken(t), nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	b := decodeList(t, rec)
	if b.Items == nil || len(b.Items) != 0 || b.Total != 0 || b.Page != 1 || b.PageSize != 20 {
		t.Errorf("expected empty page with defaults, got %+v", b)
	}
}

func TestExpenses_List_FilterByDateRange_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	createExpense(t, router, tok, map[string]any{"description": "Jan", "amountCents": 100, "dueDate": "2026-01-15T00:00:00Z"})
	createExpense(t, router, tok, map[string]any{"description": "Feb", "amountCents": 200, "dueDate": "2026-02-15T00:00:00Z"})
	createExpense(t, router, tok, map[string]any{"description": "Mar", "amountCents": 300, "dueDate": "2026-03-15T00:00:00Z"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/expenses?from=2026-02-01T00:00:00Z&to=2026-02-28T23:59:59Z", tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	if b := decodeList(t, rec); len(b.Items) != 1 || b.Total != 1 {
		t.Errorf("expected 1 expense in filter range, got %+v", b)
	}
}

func TestExpenses_List_InvalidFromDate_Returns400(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/expenses?from=not-a-date", validToken(t), nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid from date, got %d", rec.Code)
	}
}

// ── POST /expenses ────────────────────────────────────────────────────────────

func TestExpenses_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/expenses", validToken(t), map[string]any{
		"description": "Água",
		"amountCents": 5000,
		"dueDate":     "2026-03-31T00:00:00Z",
		"residentId":  "resident-1",
	}))

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var e map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&e)
	if e["id"] == nil || e["id"] == "" {
		t.Error("expected id in response")
	}
	if e["amountCents"] != float64(5000) {
		t.Errorf("expected amountCents 5000, got %v", e["amountCents"])
	}
}

func TestExpenses_Create_ZeroAmount_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/expenses", validToken(t), map[string]any{
		"description": "Isenção",
		"amountCents": 0,
		"dueDate":     "2026-03-31T00:00:00Z",
		"residentId":  "resident-1",
	}))

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestExpenses_Create_NegativeAmount_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/expenses", validToken(t), map[string]any{
		"description": "Taxa",
		"amountCents": -100,
		"dueDate":     "2026-03-31T00:00:00Z",
		"residentId":  "resident-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestExpenses_Create_MissingDueDate_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/expenses", validToken(t), map[string]any{
		"description": "Taxa",
		"amountCents": 1000,
		"residentId":  "resident-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestExpenses_Create_MissingUnitLink_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/expenses", validToken(t), map[string]any{
		"description": "Taxa",
		"amountCents": 1000,
		"dueDate":     "2026-03-31T00:00:00Z",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

// ── GET /expenses/{id} ────────────────────────────────────────────────────────

func TestExpenses_GetByID_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	e := createExpense(t, router, tok, nil)
	id := fmt.Sprintf("%v", e["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/expenses/"+id, tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestExpenses_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/expenses/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /expenses/{id} ────────────────────────────────────────────────────────

func TestExpenses_Update_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	e := createExpense(t, router, tok, nil)
	id := fmt.Sprintf("%v", e["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/expenses/"+id, tok, map[string]any{
		"description": "Taxa Atualizada",
		"amountCents": 20000,
		"dueDate":     "2026-04-30T00:00:00Z",
	}))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["amountCents"] != float64(20000) {
		t.Errorf("expected amountCents 20000, got %v", updated["amountCents"])
	}
}

func TestExpenses_Update_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/expenses/ghost-id", validToken(t), map[string]any{
		"description": "X",
		"amountCents": 100,
	}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── DELETE /expenses/{id} ─────────────────────────────────────────────────────

func TestExpenses_Delete_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	e := createExpense(t, router, tok, nil)
	id := fmt.Sprintf("%v", e["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/expenses/"+id, tok, nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestExpenses_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/expenses/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func TestIntegration_Expenses_CRUD(t *testing.T) {
	pool := dbtest.NewPool(t)
	router := expense.NewHandler(expense.NewService(expense.NewPostgresRepository(pool), testUnits), middleware.Authenticate(handlerJWT))

	crudRoundTrip(t, router, validToken(t), "/expenses",
		map[string]any{"description": "Taxa", "amountCents": 15000, "dueDate": "2026-03-31T00:00:00Z", "residentId": "resident-1"},
		map[string]any{"amountCents": 20000},
		func(_, updated map[string]any) {
			if updated["amountCents"] != float64(20000) {
				t.Errorf("update not persisted: %v", updated)
			}
			if updated["dueDate"] != "2026-03-31T00:00:00Z" {
				t.Errorf("expected dueDate in UTC, got %v", updated["dueDate"])
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
	if b := decodeList(t, rec); len(b.Items) != 1 || b.Total != 1 {
		t.Fatalf("list: expected 1 item, got %+v", b)
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

// ── Security ──────────────────────────────────────────────────────────────────

func serve(router http.Handler, method, path, tok string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(method, path, tok, body))
	return rec
}

func TestHandler_Security_InvalidToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	other := pkgjwt.MustNewService("b3RoZXItc2VjcmV0LWtleS1mb3ItdW5pdC10ZXN0aW5nLTMyYg==")
	forged, _ := other.GenerateToken("admin-1", "condo-a", []string{authz.RoleAdmin})
	if rec := serve(router, http.MethodGet, "/expenses", forged, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("token signed with another key: expected 401, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPost, "/expenses", "", map[string]any{"description": "x"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("create without token: expected 401, got %d", rec.Code)
	}
}

func TestHandler_Security_UserCannotCreateUpdateDelete_Returns403(t *testing.T) {
	router := newTestRouter(t)
	e := createExpense(t, router, validToken(t), map[string]any{"residentId": "res-a"})
	id := fmt.Sprintf("%v", e["id"])
	user := tokenFor(t, "user-a", "condo-a", authz.RoleUser) // owner of res-a

	if rec := serve(router, http.MethodPost, "/expenses", user, map[string]any{
		"description": "x", "amountCents": 1, "dueDate": "2026-03-31T00:00:00Z", "residentId": "res-a",
	}); rec.Code != http.StatusForbidden {
		t.Errorf("create: expected 403, got %d — %s", rec.Code, rec.Body.String())
	}
	if rec := serve(router, http.MethodPut, "/expenses/"+id, user, map[string]any{"amountCents": 1}); rec.Code != http.StatusForbidden {
		t.Errorf("update: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, "/expenses/"+id, user, nil); rec.Code != http.StatusForbidden {
		t.Errorf("delete: expected 403, got %d", rec.Code)
	}
	// A token without any role is not an admin either.
	if rec := serve(router, http.MethodDelete, "/expenses/"+id, tokenFor(t, "user-x", "condo-a"), nil); rec.Code != http.StatusForbidden {
		t.Errorf("delete without roles: expected 403, got %d", rec.Code)
	}

	rec := serve(router, http.MethodGet, "/expenses/"+id, validToken(t), nil)
	var got map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if rec.Code != http.StatusOK || got["amountCents"] != e["amountCents"] {
		t.Errorf("expense must be untouched: %d %v", rec.Code, got)
	}
}

func TestHandler_Security_UserSeesOnlyOwnUnits(t *testing.T) {
	router := newTestRouter(t)
	admin := validToken(t)
	resA := createExpense(t, router, admin, map[string]any{"description": "res-a", "dueDate": "2026-01-01T00:00:00Z", "residentId": "res-a"})
	shopA := createExpense(t, router, admin, map[string]any{"description": "shop-a", "dueDate": "2026-02-01T00:00:00Z", "residentId": "", "shopOwnerId": "shop-a"})
	resB := createExpense(t, router, admin, map[string]any{"description": "res-b", "dueDate": "2026-03-01T00:00:00Z", "residentId": "res-b"})
	userA := tokenFor(t, "user-a", "condo-a", authz.RoleUser)
	userB := tokenFor(t, "user-b", "condo-a", authz.RoleUser)

	b := decodeList(t, serve(router, http.MethodGet, "/expenses", userA, nil))
	if b.Total != 2 || len(b.Items) != 2 || b.Items[0]["id"] != resA["id"] || b.Items[1]["id"] != shopA["id"] {
		t.Fatalf("user-a list: expected [res-a shop-a], got %+v", b)
	}
	b = decodeList(t, serve(router, http.MethodGet, "/expenses", userB, nil))
	if b.Total != 1 || len(b.Items) != 1 || b.Items[0]["id"] != resB["id"] {
		t.Fatalf("user-b list: expected [res-b], got %+v", b)
	}
	b = decodeList(t, serve(router, http.MethodGet, "/expenses", tokenFor(t, "nobody", "condo-a", authz.RoleUser), nil))
	if b.Total != 0 || len(b.Items) != 0 {
		t.Fatalf("user without units: expected empty list, got %+v", b)
	}
	if b = decodeList(t, serve(router, http.MethodGet, "/expenses", admin, nil)); b.Total != 3 {
		t.Errorf("admin list: expected 3, got %+v", b)
	}

	if rec := serve(router, http.MethodGet, fmt.Sprintf("/expenses/%v", shopA["id"]), userA, nil); rec.Code != http.StatusOK {
		t.Errorf("user-a GET own shop expense: expected 200, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, fmt.Sprintf("/expenses/%v", resB["id"]), userA, nil); rec.Code != http.StatusNotFound {
		t.Errorf("user-a GET user-b expense: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, fmt.Sprintf("/expenses/%v", resA["id"]), userB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("user-b GET user-a expense: expected 404, got %d", rec.Code)
	}
}

func TestHandler_Security_OtherCondominiumToken_Returns404(t *testing.T) {
	router := newTestRouter(t)
	e := createExpense(t, router, validToken(t), map[string]any{"residentId": "res-a"})
	path := fmt.Sprintf("/expenses/%v", e["id"])
	otherAdmin := tokenFor(t, "admin-1", "condo-b", authz.RoleAdmin) // same user ID, other condominium

	if rec := serve(router, http.MethodGet, path, otherAdmin, nil); rec.Code != http.StatusNotFound {
		t.Errorf("get: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, path, otherAdmin, map[string]any{"amountCents": 1}); rec.Code != http.StatusNotFound {
		t.Errorf("update: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, path, otherAdmin, nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", rec.Code)
	}
	if b := decodeList(t, serve(router, http.MethodGet, "/expenses", otherAdmin, nil)); b.Total != 0 || len(b.Items) != 0 {
		t.Errorf("list: expected nothing from condo-a, got %+v", b)
	}
	// user-a's units resolve to res-a, but the token is for condo-b.
	if rec := serve(router, http.MethodGet, path, tokenFor(t, "user-a", "condo-b", authz.RoleUser), nil); rec.Code != http.StatusNotFound {
		t.Errorf("user of other condominium: expected 404, got %d", rec.Code)
	}

	rec := serve(router, http.MethodGet, path, validToken(t), nil)
	var got map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if rec.Code != http.StatusOK || got["amountCents"] != e["amountCents"] {
		t.Errorf("condo-a expense must be untouched: %d %v", rec.Code, got)
	}
}

func TestHandler_List_Pagination(t *testing.T) {
	router := newTestRouter(t)
	admin := validToken(t)
	for _, d := range []string{"2026-01-10", "2026-02-10", "2026-02-20", "2026-02-05", "2026-02-15", "2026-03-10"} {
		createExpense(t, router, admin, map[string]any{"description": d, "dueDate": d + "T00:00:00Z"})
	}

	b := decodeList(t, serve(router, http.MethodGet, "/expenses?page=2&pageSize=4", admin, nil))
	if b.Page != 2 || b.PageSize != 4 || b.Total != 6 || len(b.Items) != 2 ||
		b.Items[0]["description"] != "2026-02-20" || b.Items[1]["description"] != "2026-03-10" {
		t.Fatalf("page 2: unexpected %+v", b)
	}

	// Combined with the date filter: February has 4 expenses.
	q := "/expenses?from=2026-02-01T00:00:00Z&to=2026-02-28T23:59:59Z&pageSize=3"
	b = decodeList(t, serve(router, http.MethodGet, q, admin, nil))
	if b.Page != 1 || b.PageSize != 3 || b.Total != 4 || len(b.Items) != 3 ||
		b.Items[0]["description"] != "2026-02-05" || b.Items[2]["description"] != "2026-02-15" {
		t.Fatalf("filtered page 1: unexpected %+v", b)
	}
	b = decodeList(t, serve(router, http.MethodGet, q+"&page=2", admin, nil))
	if b.Total != 4 || len(b.Items) != 1 || b.Items[0]["description"] != "2026-02-20" {
		t.Fatalf("filtered page 2: unexpected %+v", b)
	}
	b = decodeList(t, serve(router, http.MethodGet, q+"&page=5", admin, nil))
	if b.Total != 4 || b.Items == nil || len(b.Items) != 0 {
		t.Fatalf("past the end: expected empty items, got %+v", b)
	}

	if b = decodeList(t, serve(router, http.MethodGet, "/expenses?pageSize=100", admin, nil)); b.PageSize != 100 || len(b.Items) != 6 {
		t.Errorf("max pageSize: unexpected %+v", b)
	}
	for _, bad := range []string{"page=0", "page=-1", "page=abc", "pageSize=0", "pageSize=101", "pageSize=x"} {
		if rec := serve(router, http.MethodGet, "/expenses?"+bad, admin, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", bad, rec.Code)
		}
	}
}

func TestHandler_List_InvalidToDate_Returns400(t *testing.T) {
	router := newTestRouter(t)
	if rec := serve(router, http.MethodGet, "/expenses?to=yesterday", validToken(t), nil); rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestIntegration_Expenses_Security(t *testing.T) {
	pool := dbtest.NewPool(t)
	router := expense.NewHandler(expense.NewService(expense.NewPostgresRepository(pool), testUnits), middleware.Authenticate(handlerJWT))
	admin := validToken(t)
	resA := createExpense(t, router, admin, map[string]any{"residentId": "res-a"})
	resB := createExpense(t, router, admin, map[string]any{"residentId": "res-b"})
	userA := tokenFor(t, "user-a", "condo-a", authz.RoleUser)

	if b := decodeList(t, serve(router, http.MethodGet, "/expenses", userA, nil)); b.Total != 1 || len(b.Items) != 1 || b.Items[0]["id"] != resA["id"] {
		t.Errorf("user-a list: expected only res-a, got %+v", b)
	}
	if rec := serve(router, http.MethodGet, fmt.Sprintf("/expenses/%v", resB["id"]), userA, nil); rec.Code != http.StatusNotFound {
		t.Errorf("user-a GET res-b: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, fmt.Sprintf("/expenses/%v", resA["id"]), userA, nil); rec.Code != http.StatusForbidden {
		t.Errorf("user-a DELETE: expected 403, got %d", rec.Code)
	}
	otherAdmin := tokenFor(t, "admin-1", "condo-b", authz.RoleAdmin)
	if rec := serve(router, http.MethodPut, fmt.Sprintf("/expenses/%v", resA["id"]), otherAdmin, map[string]any{"amountCents": 1}); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b admin PUT: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, fmt.Sprintf("/expenses/%v", resA["id"]), otherAdmin, nil); rec.Code != http.StatusNotFound {
		t.Errorf("condo-b admin GET: expected 404, got %d", rec.Code)
	}
	if b := decodeList(t, serve(router, http.MethodGet, "/expenses", otherAdmin, nil)); b.Total != 0 {
		t.Errorf("condo-b admin list: expected nothing, got %+v", b)
	}
}
