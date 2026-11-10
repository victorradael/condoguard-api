package notification_test

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
	"github.com/victorradael/condoguard/api/internal/notification"
	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

const handlerSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"

var handlerJWT = pkgjwt.MustNewService(handlerSecret)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	svc := notification.NewService(notification.NewInMemoryRepository(), testUnits)
	return notification.NewHandler(svc, middleware.Authenticate(handlerJWT))
}

func tokenFor(t *testing.T, userID, condo string, roles ...string) string {
	t.Helper()
	tok, err := handlerJWT.GenerateToken(userID, condo, roles)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

// validToken is an admin of testCondo (admins manage notifications).
func validToken(t *testing.T) string {
	t.Helper()
	return tokenFor(t, "admin-1", testCondo, authz.RoleSindico)
}

func userToken(t *testing.T, userID string) string {
	t.Helper()
	return tokenFor(t, userID, testCondo, authz.RoleMorador)
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

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

type listBody struct {
	Items    []map[string]any `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int64            `json:"total"`
}

func createNotification(t *testing.T, router http.Handler, tok string, extra map[string]any) map[string]any {
	t.Helper()
	payload := map[string]any{
		"message":     "Aviso de teste",
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
	list := decode[listBody](t, rec)
	if list.Items == nil || len(list.Items) != 0 || list.Total != 0 {
		t.Errorf("expected empty items array, got %+v", list)
	}
}

// ── POST /notifications ───────────────────────────────────────────────────────

func TestNotifications_Create_Returns201(t *testing.T) {
	router := newTestRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, authReq(http.MethodPost, "/notifications", validToken(t), map[string]any{
		"message":     "Reunião geral",
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
		"message": "Aviso",
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

// ── Integration (PostgreSQL; requires TEST_DATABASE_URL) ───────────────────────

func TestIntegration_Notifications_CRUD(t *testing.T) {
	pool := dbtest.NewPool(t, dbtest.CondoA, dbtest.CondoB)
	const condo = dbtest.CondoA
	admin := saveUser(t, pool, condo, "admin", authz.RoleSindico)
	router := tenant.Middleware("deployment-default")(notification.NewHandler(
		notification.NewService(notification.NewPostgresRepository(pool), testUnits),
		middleware.Authenticate(handlerJWT)))
	tok := tokenFor(t, admin, condo, authz.RoleSindico)

	crudRoundTrip(t, router, tok, "/notifications",
		map[string]any{"message": "Aviso", "residentIds": []string{"r1"}, "createdById": "someone-else"},
		map[string]any{"message": "Aviso atualizado"},
		func(created, updated map[string]any) {
			if created["createdById"] != admin || updated["createdById"] != admin {
				t.Errorf("createdById must be the token user %q: created=%v updated=%v", admin, created["createdById"], updated["createdById"])
			}
			if updated["message"] != "Aviso atualizado" {
				t.Errorf("update not persisted: %v", updated)
			}
			if ids, _ := updated["shopOwnerIds"].([]any); ids == nil {
				t.Errorf("shopOwnerIds must be an empty array, got %v", updated["shopOwnerIds"])
			}
			// mark read through the handler (needs a real user for the FK)
			id, _ := created["id"].(string)
			rec := serve(router, http.MethodPut, "/notifications/"+id+"/read", tok, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("mark read: expected 200, got %d — %s", rec.Code, rec.Body.String())
			}
			if n := decode[map[string]any](t, rec); n["read"] != true || n["readAt"] == nil {
				t.Errorf("mark read: %v", n)
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
	var list listBody
	_ = json.NewDecoder(rec.Body).Decode(&list)
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

// ── Security ──────────────────────────────────────────────────────────────────

func TestHandler_Security_CreatedByIDComesFromToken(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)

	n := createNotification(t, router, tok, map[string]any{"createdById": "someone-else"})
	if n["createdById"] != "admin-1" {
		t.Errorf("create response: createdById must be the token user, got %v", n["createdById"])
	}
	rec := serve(router, http.MethodGet, fmt.Sprintf("/notifications/%v", n["id"]), tok, nil)
	if got := decode[map[string]any](t, rec); got["createdById"] != "admin-1" {
		t.Errorf("stored createdById must be the token user, got %v", got["createdById"])
	}
}

func TestHandler_Security_ReadStateIsPerRecipient(t *testing.T) {
	router := newTestRouter(t)
	n := createNotification(t, router, validToken(t), nil) // addressed to r1
	path := fmt.Sprintf("/notifications/%v", n["id"])
	tokA, tokB := userToken(t, "res-1"), userToken(t, "res-2") // both live in r1

	rec := serve(router, http.MethodPut, path+"/read", tokA, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("A mark read: expected 200, got %d — %s", rec.Code, rec.Body.String())
	}
	first := decode[map[string]any](t, rec)
	if first["read"] != true || first["readAt"] == nil {
		t.Fatalf("A mark read: %v", first)
	}

	if got := decode[map[string]any](t, serve(router, http.MethodGet, path, tokA, nil)); got["read"] != true || got["readAt"] != first["readAt"] {
		t.Errorf("A must see read=true with the same readAt: %v", got)
	}
	if got := decode[map[string]any](t, serve(router, http.MethodGet, path, tokB, nil)); got["read"] != false || got["readAt"] != nil {
		t.Errorf("B must still see read=false: %v", got)
	}
	list := decode[listBody](t, serve(router, http.MethodGet, "/notifications", tokB, nil))
	if len(list.Items) != 1 || list.Items[0]["read"] != false {
		t.Errorf("B list must show unread: %+v", list)
	}

	// idempotent: A marks again, readAt unchanged
	again := decode[map[string]any](t, serve(router, http.MethodPut, path+"/read", tokA, nil))
	if again["read"] != true || again["readAt"] != first["readAt"] {
		t.Errorf("idempotent mark read changed readAt: first=%v again=%v", first["readAt"], again["readAt"])
	}
}

func TestHandler_Security_UserCannotManage_Returns403(t *testing.T) {
	router := newTestRouter(t)
	n := createNotification(t, router, validToken(t), nil)
	path := fmt.Sprintf("/notifications/%v", n["id"])
	tok := userToken(t, "res-1") // a recipient, but not an admin

	if rec := serve(router, http.MethodPost, "/notifications", tok, map[string]any{"message": "X", "residentIds": []string{"r1"}}); rec.Code != http.StatusForbidden {
		t.Errorf("create: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, path, tok, map[string]any{"message": "X"}); rec.Code != http.StatusForbidden {
		t.Errorf("update: expected 403, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, path, tok, nil); rec.Code != http.StatusForbidden {
		t.Errorf("delete: expected 403, got %d", rec.Code)
	}
	got := decode[map[string]any](t, serve(router, http.MethodGet, path, validToken(t), nil))
	if got["message"] != "Aviso de teste" {
		t.Errorf("notification must be untouched, got %v", got)
	}
}

func TestHandler_Security_UserNotAddressed_Returns404(t *testing.T) {
	router := newTestRouter(t)
	n := createNotification(t, router, validToken(t), nil) // addressed to r1
	path := fmt.Sprintf("/notifications/%v", n["id"])
	tok := userToken(t, "outsider") // owns r9

	if rec := serve(router, http.MethodGet, path, tok, nil); rec.Code != http.StatusNotFound {
		t.Errorf("get: expected 404, got %d", rec.Code)
	}
	if rec := serve(router, http.MethodPut, path+"/read", tok, nil); rec.Code != http.StatusNotFound {
		t.Errorf("mark read: expected 404, got %d", rec.Code)
	}
	rec := serve(router, http.MethodGet, "/notifications", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	if list := decode[listBody](t, rec); len(list.Items) != 0 || list.Total != 0 {
		t.Errorf("list: notification must be absent, got %+v", list)
	}

	// an addressed user does see it
	if rec := serve(router, http.MethodGet, path, userToken(t, "res-1"), nil); rec.Code != http.StatusOK {
		t.Errorf("addressed user get: expected 200, got %d", rec.Code)
	}
}

func TestHandler_Security_OtherCondominium_Returns404EvenForAdmin(t *testing.T) {
	router := newTestRouter(t)
	n := createNotification(t, router, validToken(t), nil)
	path := fmt.Sprintf("/notifications/%v", n["id"])
	other := tokenFor(t, "admin-b", dbtest.CondoB, authz.RoleSindico)

	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, path, nil},
		{http.MethodPut, path, map[string]any{"message": "X"}},
		{http.MethodPut, path + "/read", nil},
		{http.MethodDelete, path, nil},
	} {
		if rec := serve(router, c.method, c.path, other, c.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: expected 404, got %d", c.method, c.path, rec.Code)
		}
	}
	if list := decode[listBody](t, serve(router, http.MethodGet, "/notifications", other, nil)); len(list.Items) != 0 || list.Total != 0 {
		t.Errorf("list: expected empty, got %+v", list)
	}
	if got := decode[map[string]any](t, serve(router, http.MethodGet, path, validToken(t), nil)); got["message"] != "Aviso de teste" {
		t.Errorf("notification must be untouched, got %v", got)
	}
}

func TestHandler_Pagination(t *testing.T) {
	router := newTestRouter(t)
	tok := validToken(t)
	for i := range 3 {
		createNotification(t, router, tok, map[string]any{"message": fmt.Sprintf("Aviso %d", i)})
	}

	rec := serve(router, http.MethodGet, "/notifications?page=2&pageSize=2", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	list := decode[listBody](t, rec)
	if list.Page != 2 || list.PageSize != 2 || list.Total != 3 || len(list.Items) != 1 {
		t.Errorf("unexpected page: %+v", list)
	}

	def := decode[listBody](t, serve(router, http.MethodGet, "/notifications", tok, nil))
	if def.Page != 1 || def.PageSize != 20 || def.Total != 3 || len(def.Items) != 3 {
		t.Errorf("default page: %+v", def)
	}

	for _, q := range []string{"page=0", "page=x", "pageSize=0", "pageSize=101", "pageSize=-1"} {
		if rec := serve(router, http.MethodGet, "/notifications?"+q, tok, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, rec.Code)
		}
	}
}
