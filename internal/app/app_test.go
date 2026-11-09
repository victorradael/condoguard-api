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

const testSecret = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"

// clearEnv blanks every setting and sets only the required JWT secret.
func clearEnv(t *testing.T) {
	for _, k := range []string{
		"DATABASE_URL", "DATABASE_MIGRATIONS_URL", "REDIS_URL", "DB_MIGRATE_ON_START", "DEFAULT_CONDOMINIUM_ID",
		"LOGIN_MAX_ATTEMPTS", "LOGIN_MAX_ATTEMPTS_PER_IP", "LOGIN_LOCKOUT_WINDOW",
		"REGISTER_MAX_PER_IP", "REGISTER_WINDOW",
		"ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL", "CORS_ALLOWED_ORIGINS",
		"COOKIE_SECURE", "COOKIE_DOMAIN", "COOKIE_PATH_PREFIX", "METRICS_TOKEN",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("JWT_SECRET_KEY", testSecret)
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
	if cfg.RegisterMaxPerIP != 10 || cfg.RegisterWindow != time.Hour {
		t.Errorf("sign-up limit must be on by default: %+v", cfg)
	}
	if !cfg.CookieSecure || cfg.MetricsToken != "" || len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("unexpected security defaults: %+v", cfg)
	}
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("DEFAULT_CONDOMINIUM_ID", "condo-x")
	t.Setenv("DB_MIGRATE_ON_START", "false")
	t.Setenv("LOGIN_MAX_ATTEMPTS", "3")
	t.Setenv("LOGIN_LOCKOUT_WINDOW", "2m")
	t.Setenv("COOKIE_PATH_PREFIX", "/api")
	t.Setenv("DATABASE_MIGRATIONS_URL", "postgres://owner@db/x")
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CondominiumID != "condo-x" || cfg.MigrateOnStart || cfg.LoginMaxAttempts != 3 || cfg.LoginLockoutWindow != 2*time.Minute || cfg.CookiePathPrefix != "/api" || cfg.DatabaseMigrationsURL != "postgres://owner@db/x" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

// Item 2: the API must not start without a strong JWT secret.
func TestConfigFromEnv_Security_RejectsMissingOrWeakJWTSecret(t *testing.T) {
	for name, v := range map[string]string{
		"missing":        "",
		"old default":    "secret",
		"not base64":     "this is not base64 but is long enough to pass a length check!",
		"under 32 bytes": "c2hvcnQtc2VjcmV0LTMxLWJ5dGVzLWxvbmctLi4uLg==",
	} {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("JWT_SECRET_KEY", v)
			if _, err := app.ConfigFromEnv(); err == nil {
				t.Errorf("JWT_SECRET_KEY=%q must be rejected", v)
			}
		})
	}
}

func TestNew_Security_RejectsWeakJWTSecret(t *testing.T) {
	cfg := app.Config{JWTSecret: "secret", CondominiumID: "c", LoginMaxAttempts: 5, LoginLockoutWindow: time.Minute}
	if _, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics()); err == nil {
		t.Fatal("New must refuse a weak JWT secret")
	}
}

func TestConfigFromEnv_Invalid(t *testing.T) {
	for k, v := range map[string]string{
		"LOGIN_MAX_ATTEMPTS":   "0",
		"LOGIN_LOCKOUT_WINDOW": "soon",
		"DB_MIGRATE_ON_START":  "maybe",
		"CORS_ALLOWED_ORIGINS": "*",
		"METRICS_TOKEN":        "short",
		"ACCESS_TOKEN_TTL":     "-1m",
		"COOKIE_SECURE":        "perhaps",
		"REGISTER_MAX_PER_IP":  "0",
		"REGISTER_WINDOW":      "0s",
		"COOKIE_PATH_PREFIX":   "/api/",
	} {
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
	cfg := app.Config{DatabaseURL: "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=1", JWTSecret: testSecret, CondominiumID: "c", LoginMaxAttempts: 5, LoginLockoutWindow: time.Minute}
	if _, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics()); err == nil {
		t.Fatal("expected an error for an unreachable database")
	}
}

// A malformed REDIS_URL must not put the Redis password in the startup log.
func TestNew_Security_InvalidRedisURLHidesPassword(t *testing.T) {
	cfg := app.Config{RedisURL: "redis://:s3cr3t-pass@host:notaport/0", JWTSecret: testSecret, CondominiumID: "c", LoginMaxAttempts: 5, LoginLockoutWindow: time.Minute}
	_, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics())
	if err == nil {
		t.Fatal("expected an error for a malformed REDIS_URL")
	}
	if strings.Contains(err.Error(), "s3cr3t-pass") {
		t.Errorf("error leaks the Redis password: %v", err)
	}
}

func TestRouter_LoginIsRateLimited(t *testing.T) {
	cfg := app.Config{JWTSecret: testSecret, CondominiumID: "c", LoginMaxAttempts: 2, LoginLockoutWindow: time.Minute}
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
		if code := post("/auth/login", `{"email":"ana@exemplo.test","password":"wrong"}`); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, code)
		}
	}
	if code := post("/auth/login", `{"email":"ana@exemplo.test","password":"S3cr3t!Pass"}`); code != http.StatusTooManyRequests {
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
