package openapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/pkg/openapi"
)

// ── Spec structure ─────────────────────────────────────────────────────────

func TestSpec_HasCorrectOpenAPIVersion(t *testing.T) {
	spec := openapi.NewSpec()
	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %q", spec.OpenAPI)
	}
}

func TestSpec_HasInfoBlock(t *testing.T) {
	spec := openapi.NewSpec()
	if spec.Info.Title == "" {
		t.Error("expected non-empty info.title")
	}
	if spec.Info.Version == "" {
		t.Error("expected non-empty info.version")
	}
}

func TestSpec_HasSecurityScheme(t *testing.T) {
	spec := openapi.NewSpec()
	scheme, ok := spec.Components.SecuritySchemes["bearerAuth"]
	if !ok {
		t.Fatal("expected bearerAuth security scheme")
	}
	if scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Errorf("expected http/bearer scheme, got type=%q scheme=%q", scheme.Type, scheme.Scheme)
	}
}

func TestSpec_ContainsAllExpectedPaths(t *testing.T) {
	spec := openapi.NewSpec()

	required := []string{
		"/health",
		"/auth/register",
		"/auth/login",
		"/users",
		"/users/{id}",
		"/residents",
		"/residents/{id}",
		"/shopOwners",
		"/shopOwners/{id}",
		"/expenses",
		"/expenses/{id}",
		"/notifications",
		"/notifications/{id}",
		"/notifications/{id}/read",
	}

	for _, path := range required {
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("missing path %q in spec", path)
		}
	}
}

func TestSpec_AuthRoutes_ArePublic(t *testing.T) {
	spec := openapi.NewSpec()

	for _, method := range []string{"post"} {
		op := spec.Paths["/auth/register"][method]
		if op == nil {
			t.Fatal("missing POST /auth/register")
		}
		if len(op.Security) != 0 {
			t.Error("POST /auth/register must not require authentication")
		}
	}
}

func TestSpec_ProtectedRoutes_RequireBearerAuth(t *testing.T) {
	spec := openapi.NewSpec()

	protected := map[string]string{
		"/users":         "get",
		"/residents":     "get",
		"/shopOwners":    "get",
		"/expenses":      "get",
		"/notifications": "get",
	}

	for path, method := range protected {
		op := spec.Paths[path][method]
		if op == nil {
			t.Errorf("missing %s %s", strings.ToUpper(method), path)
			continue
		}
		found := false
		for _, s := range op.Security {
			if _, ok := s["bearerAuth"]; ok {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s %s must require bearerAuth", strings.ToUpper(method), path)
		}
	}
}

func TestSpec_AllSchemas_Referenced(t *testing.T) {
	spec := openapi.NewSpec()

	required := []string{
		"Error", "RegisterRequest", "LoginRequest", "LoginResponse",
		"User", "CreateUserRequest", "UpdateUserRequest",
		"Resident", "CreateResidentRequest", "UpdateResidentRequest",
		"ShopOwner", "CreateShopOwnerRequest", "UpdateShopOwnerRequest",
		"Expense", "CreateExpenseRequest", "UpdateExpenseRequest",
		"Notification", "CreateNotificationRequest", "UpdateNotificationRequest",
	}

	for _, name := range required {
		if _, ok := spec.Components.Schemas[name]; !ok {
			t.Errorf("missing schema %q in components", name)
		}
	}
}

func TestSpec_IsValidJSON(t *testing.T) {
	spec := openapi.NewSpec()
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("spec must marshal to valid JSON: %v", err)
	}
	if len(b) == 0 {
		t.Error("marshaled spec must not be empty")
	}
}

// ── HTTP handlers ──────────────────────────────────────────────────────────

func TestHandler_OpenAPIJSON_Returns200WithJSON(t *testing.T) {
	h := openapi.Handler()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}

	var v map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if v["openapi"] != "3.1.0" {
		t.Errorf("expected openapi 3.1.0, got %v", v["openapi"])
	}
}

func TestHandler_SwaggerUI_Returns200WithHTML(t *testing.T) {
	h := openapi.UIHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("expected swagger-ui in HTML body")
	}
	if !strings.Contains(body, "/openapi.json") {
		t.Error("expected /openapi.json reference in swagger UI HTML")
	}
}
