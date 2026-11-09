package app_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/app"
	"github.com/victorradael/condoguard/api/internal/middleware"
)

func clearEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "REDIS_URL", "DB_MIGRATE_ON_START", "DEFAULT_CONDOMINIUM_ID", "LOGIN_MAX_ATTEMPTS", "LOGIN_LOCKOUT_WINDOW"} {
		t.Setenv(k, "")
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	clearEnv(t)
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CondominiumID != "default" || !cfg.MigrateOnStart || cfg.LoginMaxAttempts != 5 || cfg.LoginLockoutWindow != 15*time.Minute {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("DEFAULT_CONDOMINIUM_ID", "condo-x")
	t.Setenv("DB_MIGRATE_ON_START", "false")
	t.Setenv("LOGIN_MAX_ATTEMPTS", "3")
	t.Setenv("LOGIN_LOCKOUT_WINDOW", "2m")
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CondominiumID != "condo-x" || cfg.MigrateOnStart || cfg.LoginMaxAttempts != 3 || cfg.LoginLockoutWindow != 2*time.Minute {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestConfigFromEnv_Invalid(t *testing.T) {
	for k, v := range map[string]string{"LOGIN_MAX_ATTEMPTS": "0", "LOGIN_LOCKOUT_WINDOW": "soon", "DB_MIGRATE_ON_START": "maybe"} {
		t.Run(k, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(k, v)
			if _, err := app.ConfigFromEnv(); err == nil {
				t.Errorf("%s=%q must be rejected", k, v)
			}
		})
	}
}

func TestNew_UnreachableDatabase_ReturnsError(t *testing.T) {
	cfg := app.Config{DatabaseURL: "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1", CondominiumID: "c", LoginMaxAttempts: 5, LoginLockoutWindow: time.Minute}
	if _, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics()); err == nil {
		t.Fatal("expected an error for an unreachable database")
	}
}

func TestRouter_LoginIsRateLimited(t *testing.T) {
	cfg := app.Config{CondominiumID: "c", LoginMaxAttempts: 2, LoginLockoutWindow: time.Minute}
	a, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	post := func(path, body string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		a.Handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post("/auth/register", `{"username":"ana","email":"ana@exemplo.test","password":"S3cr3t!Pass"}`); code != http.StatusCreated {
		t.Fatalf("register: %d", code)
	}
	for i := 0; i < 2; i++ {
		if code := post("/auth/login", `{"username":"ana","password":"wrong"}`); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, code)
		}
	}
	if code := post("/auth/login", `{"username":"ana","password":"S3cr3t!Pass"}`); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after max failures, got %d", code)
	}
	// Register is not limited.
	if code := post("/auth/register", `{"username":"bia","email":"bia@exemplo.test","password":"S3cr3t!Pass"}`); code != http.StatusCreated {
		t.Errorf("register must not be limited: %d", code)
	}
	// Other methods on /auth/login keep their previous answer.
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /auth/login: expected 405, got %d", rec.Code)
	}
}
