package notification_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/notification"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3Rpbmc="

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	repo := notification.NewInMemoryRepository()
	svc := notification.NewService(repo)
	jwtSvc := pkgjwt.NewService(handlerSecret)
	return notification.NewHandler(svc, middleware.Authenticate(jwtSvc))
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

func createNotification(t *testing.T, router http.Handler, tok string, extra map[string]any) map[string]any {
	t.Helper()
	payload := map[string]any{
		"message":     "Aviso de teste",
		"createdById": "user-1",
		"residentIds": []string{"r1"},
	}
	for k, v := range extra {
		payload[k] = v
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/notifications", tok, payload))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup createNotification: expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var n map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&n)
	return n
}

// ── Auth guard ────────────────────────────────────────────────────────────────

func TestNotifications_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/notifications", "", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

// ── GET /notifications ────────────────────────────────────────────────────────

func TestNotifications_List_Empty_Returns200WithEmptyArray(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/notifications", validToken(t), nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var list []any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("expected empty array, got %v", list)
	}
}

// ── POST /notifications ───────────────────────────────────────────────────────

func TestNotifications_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/notifications", validToken(t), map[string]any{
		"message":     "Reunião geral",
		"createdById": "user-1",
		"residentIds": []string{"r1"},
	}))

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — %s", rec.Code, rec.Body.String())
	}
	var n map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&n)
	if n["id"] == nil || n["id"] == "" {
		t.Error("expected id in response")
	}
	if n["read"] != false {
		t.Error("new notification must have read=false")
	}
	if n["createdAt"] == nil {
		t.Error("expected createdAt in response")
	}
}

func TestNotifications_Create_MissingMessage_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/notifications", validToken(t), map[string]any{
		"createdById": "user-1",
		"residentIds": []string{"r1"},
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestNotifications_Create_NoRecipients_Returns422(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/notifications", validToken(t), map[string]any{
		"message":     "Aviso",
		"createdById": "user-1",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

// ── GET /notifications/{id} ───────────────────────────────────────────────────

func TestNotifications_GetByID_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	n := createNotification(t, router, tok, nil)
	id := fmt.Sprintf("%v", n["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/notifications/"+id, tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestNotifications_GetByID_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodGet, "/notifications/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /notifications/{id} ───────────────────────────────────────────────────

func TestNotifications_Update_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	n := createNotification(t, router, tok, nil)
	id := fmt.Sprintf("%v", n["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/notifications/"+id, tok, map[string]any{
		"message": "Mensagem atualizada",
	}))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["message"] != "Mensagem atualizada" {
		t.Errorf("expected updated message, got %v", updated["message"])
	}
}

func TestNotifications_Update_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/notifications/ghost-id", validToken(t), map[string]any{
		"message": "X",
	}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── DELETE /notifications/{id} ────────────────────────────────────────────────

func TestNotifications_Delete_Returns204(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	n := createNotification(t, router, tok, nil)
	id := fmt.Sprintf("%v", n["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/notifications/"+id, tok, nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestNotifications_Delete_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodDelete, "/notifications/ghost-id", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── PUT /notifications/{id}/read — marcar como lida ──────────────────────────

func TestNotifications_MarkAsRead_Returns200WithReadTrue(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	n := createNotification(t, router, tok, nil)
	id := fmt.Sprintf("%v", n["id"])

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/notifications/"+id+"/read", tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["read"] != true {
		t.Errorf("expected read=true, got %v", updated["read"])
	}
	if updated["readAt"] == nil {
		t.Error("expected readAt to be set")
	}
}

func TestNotifications_MarkAsRead_Idempotent_Returns200(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	n := createNotification(t, router, tok, nil)
	id := fmt.Sprintf("%v", n["id"])

	// first call
	router.ServeHTTP(httptest.NewRecorder(), authReq(http.MethodPut, "/notifications/"+id+"/read", tok, nil))

	// second call — must still return 200
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/notifications/"+id+"/read", tok, nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on idempotent MarkAsRead, got %d", rec.Code)
	}
	var updated map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated["read"] != true {
		t.Error("expected read=true on second call")
	}
}

func TestNotifications_MarkAsRead_NonExistent_Returns404(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPut, "/notifications/ghost-id/read", validToken(t), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Integration ───────────────────────────────────────────────────────────────

func TestIntegration_Notifications_CRUD(t *testing.T) {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI not set — skipping integration test")
	}
	t.Log("integration scaffold ready")
}
