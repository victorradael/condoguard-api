package resident_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/resident"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	repo := resident.NewInMemoryRepository()
	svc := resident.NewService(repo)
	jwtSvc := pkgjwt.NewService(handlerSecret)
	return resident.NewHandler(svc, middleware.Authenticate(jwtSvc))
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

func createResident(t *testing.T, router http.Handler, tok string, unitNumber, condoID string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/residents", tok, map[string]any{
		"unitNumber":    unitNumber,
		"floor":         1,
		"condominiumId": condoID,
		"ownerId":       "user-1",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup createResident: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var r map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&r)
	return r
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestResidents_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/residents", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GET /residents ────────────────────────────────────────────────────────────

func TestResidents_List_Empty_Returns200WithEmptyArray(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/residents", validToken(t), nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var list []any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected empty array, got %v", list)
	}
}

// ── POST /residents ───────────────────────────────────────────────────────────

func TestResidents_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/residents", validToken(t), map[string]any{
		"unitNumber":    "101",
		"floor":         1,
		"condominiumId": "condo-1",
		"ownerId":       "user-1",
	}))

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
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/residents", validToken(t), map[string]any{
		"floor":         1,
		"condominiumId": "condo-1",
		"ownerId":       "user-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestResidents_Create_MissingCondominiumID_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/residents", validToken(t), map[string]any{
		"unitNumber": "101",
		"floor":      1,
		"ownerId":    "user-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestResidents_Create_DuplicateUnitInCondominium_Returns409(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	createResident(t, router, tok, "101", "condo-1")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/residents", tok, map[string]any{
		"unitNumber":    "101",
		"floor":         2,
		"condominiumId": "condo-1",
		"ownerId":       "user-2",
	}))

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestResidents_Create_SameUnitDifferentCondominium_Returns201(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	createResident(t, router, tok, "101", "condo-1")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/residents", tok, map[string]any{
		"unitNumber":    "101",
		"floor":         1,
		"condominiumId": "condo-2",
		"ownerId":       "user-2",
	}))

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 for same unit in different condominium, got %d", rec.Code)
	}
}

// ── GET /residents/{id} ───────────────────────────────────────────────────────

func TestResidents_GetByID_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	r := createResident(t, router, tok, "501", "condo-1")
	id := fmt.Sprintf("%v", r["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/residents/"+id, tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestResidents_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/residents/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /residents/{id} ───────────────────────────────────────────────────────

func TestResidents_Update_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	r := createResident(t, router, tok, "601", "condo-1")
	id := fmt.Sprintf("%v", r["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/residents/"+id, tok, map[string]any{
		"unitNumber": "601",
		"floor":      9,
	}))

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
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/residents/ghost-id", validToken(t), map[string]any{
		"unitNumber": "101",
		"floor":      1,
	}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── DELETE /residents/{id} ────────────────────────────────────────────────────

func TestResidents_Delete_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	r := createResident(t, router, tok, "701", "condo-1")
	id := fmt.Sprintf("%v", r["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/residents/"+id, tok, nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestResidents_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/residents/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Integration ───────────────────────────────────────────────────────────────

func TestIntegration_Residents_CRUD(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI not set — skipping integration test")
	}
	t.Log("integration scaffold ready — wire MongoRepository to proceed")
}
