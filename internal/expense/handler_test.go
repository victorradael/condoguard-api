package expense_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/middleware"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	repo := expense.NewInMemoryRepository()
	svc := expense.NewService(repo)
	jwtSvc := pkgjwt.NewService(handlerSecret)
	return expense.NewHandler(svc, middleware.Authenticate(jwtSvc))
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
	var list []any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected empty array, got %v", list)
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
	var list []any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Errorf("expected 1 expense in filter range, got %d", len(list))
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

// ── Integration ───────────────────────────────────────────────────────────────

func TestIntegration_Expenses_CRUD(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI not set — skipping integration test")
	}
	t.Log("integration scaffold ready")
}
