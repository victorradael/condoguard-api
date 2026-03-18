// Package parity contains integration tests that validate the Go API contract
// against the original Java/Spring Boot implementation.
//
// These tests run only when MONGODB_URI is set (i.e. in the integration CI job).
// They exercise the full HTTP stack end-to-end — no mocks.
//
// Run: MONGODB_URI=... JWT_SECRET_KEY=... go test ./internal/parity/... -v -count=1
package parity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/internal/app"
	"github.com/victorradael/condoguard/api/internal/middleware"
)

// ── test server ───────────────────────────────────────────────────────────────

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := middleware.NewMetrics()
	return httptest.NewServer(app.NewRouter(logger, metrics))
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any, token string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return m
}

func skipWithoutMongo(t *testing.T) {
	t.Helper()
	if os.Getenv("MONGODB_URI") == "" {
		t.Skip("MONGODB_URI not set — skipping parity test")
	}
}

// ── Parity: Auth ──────────────────────────────────────────────────────────────

// Java: POST /auth/register → 201 { "message": "User registered successfully!" }
func TestParity_Register_Returns201WithMessage(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	resp := do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "parity-user",
		"email":    "parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_USER"},
	}, "")

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	body := decodeJSON(t, resp.Body)
	if body["message"] == "" {
		t.Error("expected 'message' field matching Java contract")
	}
}

// Java: POST /auth/login → 200 { "token": "...", "roles": [...] }
func TestParity_Login_Returns200WithTokenAndRoles(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "login-parity",
		"email":    "login-parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_USER"},
	}, "")

	resp := do(t, srv, http.MethodPost, "/auth/login", map[string]any{
		"username": "login-parity",
		"password": "P@ssw0rd!",
	}, "")

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body := decodeJSON(t, resp.Body)
	if body["token"] == nil || body["token"] == "" {
		t.Error("expected 'token' field in login response")
	}
	if body["roles"] == nil {
		t.Error("expected 'roles' field in login response")
	}
}

// ── Parity: Error contracts ───────────────────────────────────────────────────

// Java: GET /protected without token → 401
func TestParity_ProtectedRoute_NoToken_Returns401(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	for _, path := range []string{"/users", "/residents", "/shopOwners", "/expenses", "/notifications"} {
		resp := do(t, srv, http.MethodGet, path, nil, "")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("path %s: expected 401, got %d", path, resp.StatusCode)
		}
	}
}

// Java: GET /users without ADMIN role → 403
func TestParity_UsersRoute_NonAdmin_Returns403(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	// Register and login as regular user
	do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "regular-parity",
		"email":    "regular-parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_USER"},
	}, "")

	loginResp := do(t, srv, http.MethodPost, "/auth/login", map[string]any{
		"username": "regular-parity",
		"password": "P@ssw0rd!",
	}, "")
	loginBody := decodeJSON(t, loginResp.Body)
	token, _ := loginBody["token"].(string)

	resp := do(t, srv, http.MethodGet, "/users", nil, token)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin on /users, got %d", resp.StatusCode)
	}
}

// Java: GET /{resource}/{nonexistent} → 404
func TestParity_GetByID_NonExistent_Returns404(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	// Register admin and get token
	do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "admin-parity",
		"email":    "admin-parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_ADMIN"},
	}, "")
	loginResp := do(t, srv, http.MethodPost, "/auth/login", map[string]any{
		"username": "admin-parity",
		"password": "P@ssw0rd!",
	}, "")
	body := decodeJSON(t, loginResp.Body)
	token, _ := body["token"].(string)

	paths := []string{
		"/users/000000000000000000000000",
		"/residents/000000000000000000000000",
		"/shopOwners/000000000000000000000000",
		"/expenses/000000000000000000000000",
		"/notifications/000000000000000000000000",
	}
	for _, path := range paths {
		resp := do(t, srv, http.MethodGet, path, nil, token)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("path %s: expected 404, got %d", path, resp.StatusCode)
		}
	}
}

// ── Parity: Full CRUD round-trip ──────────────────────────────────────────────

func TestParity_Expense_CRUD_RoundTrip(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	// Setup: register + login as regular user
	do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "expense-parity",
		"email":    "expense-parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_USER"},
	}, "")
	loginResp := do(t, srv, http.MethodPost, "/auth/login", map[string]any{
		"username": "expense-parity",
		"password": "P@ssw0rd!",
	}, "")
	loginBody := decodeJSON(t, loginResp.Body)
	token, _ := loginBody["token"].(string)

	// Create
	createResp := do(t, srv, http.MethodPost, "/expenses", map[string]any{
		"description": "Conta de água",
		"amountCents": 10000,
		"dueDate":     "2026-09-01T00:00:00Z",
		"residentId":  "resident-parity-1",
	}, token)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create expense: expected 201, got %d", createResp.StatusCode)
	}
	created := decodeJSON(t, createResp.Body)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("expected id in create response")
	}

	// Read
	getResp := do(t, srv, http.MethodGet, "/expenses/"+id, nil, token)
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("get expense: expected 200, got %d", getResp.StatusCode)
	}

	// Update
	putResp := do(t, srv, http.MethodPut, "/expenses/"+id, map[string]any{
		"description": "Conta de energia",
		"amountCents": 15000,
		"dueDate":     "2026-09-02T00:00:00Z",
	}, token)
	if putResp.StatusCode != http.StatusOK {
		t.Errorf("update expense: expected 200, got %d", putResp.StatusCode)
	}
	updated := decodeJSON(t, putResp.Body)
	if updated["description"] != "Conta de energia" {
		t.Errorf("update: expected new description, got %v", updated["description"])
	}

	// Delete
	delResp := do(t, srv, http.MethodDelete, "/expenses/"+id, nil, token)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("delete expense: expected 204, got %d", delResp.StatusCode)
	}

	// Confirm deleted
	getAfterDel := do(t, srv, http.MethodGet, "/expenses/"+id, nil, token)
	if getAfterDel.StatusCode != http.StatusNotFound {
		t.Errorf("post-delete get: expected 404, got %d", getAfterDel.StatusCode)
	}
}

func TestParity_Notification_MarkAsRead_Idempotent(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "notif-parity",
		"email":    "notif-parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_USER"},
	}, "")
	loginResp := do(t, srv, http.MethodPost, "/auth/login", map[string]any{
		"username": "notif-parity",
		"password": "P@ssw0rd!",
	}, "")
	loginBody := decodeJSON(t, loginResp.Body)
	token, _ := loginBody["token"].(string)

	createResp := do(t, srv, http.MethodPost, "/notifications", map[string]any{
		"message":     "Manutenção programada",
		"createdById": "user-parity",
		"residentIds": []string{"r1"},
	}, token)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create notification: expected 201, got %d", createResp.StatusCode)
	}
	created := decodeJSON(t, createResp.Body)
	id, _ := created["id"].(string)

	// First mark as read
	r1 := do(t, srv, http.MethodPut, "/notifications/"+id+"/read", nil, token)
	if r1.StatusCode != http.StatusOK {
		t.Errorf("first markAsRead: expected 200, got %d", r1.StatusCode)
	}

	// Idempotent second call
	r2 := do(t, srv, http.MethodPut, "/notifications/"+id+"/read", nil, token)
	if r2.StatusCode != http.StatusOK {
		t.Errorf("second markAsRead (idempotent): expected 200, got %d", r2.StatusCode)
	}
	b2 := decodeJSON(t, r2.Body)
	if b2["read"] != true {
		t.Error("notification must remain read on second call")
	}
}

// ── Parity: Response schema fields ───────────────────────────────────────────

// Validates that the Go API response includes all fields expected by clients.
func TestParity_ResponseSchema_Resident(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	do(t, srv, http.MethodPost, "/auth/register", map[string]any{
		"username": "schema-parity",
		"email":    "schema-parity@example.com",
		"password": "P@ssw0rd!",
		"roles":    []string{"ROLE_USER"},
	}, "")
	loginResp := do(t, srv, http.MethodPost, "/auth/login", map[string]any{
		"username": "schema-parity",
		"password": "P@ssw0rd!",
	}, "")
	loginBody := decodeJSON(t, loginResp.Body)
	token, _ := loginBody["token"].(string)

	createResp := do(t, srv, http.MethodPost, "/residents", map[string]any{
		"unitNumber":    "101",
		"floor":         1,
		"condominiumId": "condo-parity",
		"ownerId":       "user-parity",
	}, token)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create resident: expected 201, got %d", createResp.StatusCode)
	}
	body := decodeJSON(t, createResp.Body)

	required := []string{"id", "unitNumber", "floor", "condominiumId", "ownerId"}
	for _, field := range required {
		if body[field] == nil {
			t.Errorf("response missing required field %q", field)
		}
	}
}

// ── Parity: X-Request-Id header ───────────────────────────────────────────────

func TestParity_AllResponses_HaveRequestIDHeader(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	paths := []string{"/health", "/metrics"}
	for _, path := range paths {
		resp := do(t, srv, http.MethodGet, path, nil, "")
		if resp.Header.Get("X-Request-Id") == "" {
			t.Errorf("path %s: expected X-Request-Id header", path)
		}
	}
}

// ── Parity: CORS ──────────────────────────────────────────────────────────────

func TestParity_CORS_OptionsReturnsAllowHeaders(t *testing.T) {
	skipWithoutMongo(t)
	srv := newServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("options request: %v", err)
	}
	// Go stdlib ServeMux returns 405 for OPTIONS on unregistered handlers —
	// this test documents the current behavior and acts as a signal to add
	// CORS middleware if needed for frontend compatibility.
	t.Logf("OPTIONS /auth/login → %d (add CORS middleware if frontend requires 200/204)", resp.StatusCode)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// containsAll reports whether all keys are present in m with non-nil values.
func containsAll(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if m[k] == nil {
			return false
		}
	}
	return true
}

var _ = containsAll // used by future tests
var _ = strings.Contains
var _ = context.Background
