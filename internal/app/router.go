// Package app wires all domain handlers into a single http.Handler.
// It is separated from cmd/server/main.go so tests can import it directly.
package app

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/victorradael/condoguard/api/internal/auth"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/notification"
	"github.com/victorradael/condoguard/api/internal/resident"
	"github.com/victorradael/condoguard/api/internal/shopowner"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/openapi"
	"github.com/victorradael/condoguard/api/pkg/ratelimit"
)

// Config holds the infrastructure settings read from the environment.
type Config struct {
	DatabaseURL        string        // DATABASE_URL; empty = in-memory repositories (dev/tests only)
	MigrateOnStart     bool          // DB_MIGRATE_ON_START (default true)
	RedisURL           string        // REDIS_URL; empty = in-memory login limiter (single instance)
	CondominiumID      string        // DEFAULT_CONDOMINIUM_ID (default "default") — tenant until 1b
	LoginMaxAttempts   int           // LOGIN_MAX_ATTEMPTS (default 5)
	LoginLockoutWindow time.Duration // LOGIN_LOCKOUT_WINDOW (default 15m)
	JWTSecret          string        // JWT_SECRET_KEY
}

// ConfigFromEnv reads Config from environment variables, applying defaults.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		MigrateOnStart:     true,
		RedisURL:           os.Getenv("REDIS_URL"),
		CondominiumID:      os.Getenv("DEFAULT_CONDOMINIUM_ID"),
		LoginMaxAttempts:   5,
		LoginLockoutWindow: 15 * time.Minute,
		JWTSecret:          os.Getenv("JWT_SECRET_KEY"),
	}
	if cfg.CondominiumID == "" {
		cfg.CondominiumID = "default"
	}
	if v := os.Getenv("DB_MIGRATE_ON_START"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("DB_MIGRATE_ON_START: %w", err)
		}
		cfg.MigrateOnStart = b
	}
	if v := os.Getenv("LOGIN_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("LOGIN_MAX_ATTEMPTS must be a positive integer")
		}
		cfg.LoginMaxAttempts = n
	}
	if v := os.Getenv("LOGIN_LOCKOUT_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return cfg, fmt.Errorf("LOGIN_LOCKOUT_WINDOW must be a positive duration (e.g. 15m)")
		}
		cfg.LoginLockoutWindow = d
	}
	return cfg, nil
}

// App owns the process-wide resources: the single pgx pool, the single Redis
// client and the HTTP handler built on top of them.
type App struct {
	Handler http.Handler
	pool    *pgxpool.Pool
	redis   *redis.Client
}

// Close releases the database pool and the Redis client.
func (a *App) Close() {
	if a.pool != nil {
		a.pool.Close()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
}

// New connects to PostgreSQL and Redis (when configured), runs the migrations
// and builds the HTTP handler.
func New(ctx context.Context, cfg Config, logger *slog.Logger, metrics *middleware.Metrics) (*App, error) {
	a := &App{}

	if cfg.DatabaseURL != "" {
		pool, err := db.NewPool(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		a.pool = pool
		if cfg.MigrateOnStart {
			if err := db.MigrateUp(ctx, pool); err != nil {
				a.Close()
				return nil, err
			}
		}
	} else {
		slog.Warn("DATABASE_URL not set — using in-memory repositories (not for production)")
	}

	var limiter ratelimit.Limiter
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("REDIS_URL: %w", err)
		}
		a.redis = redis.NewClient(opts)
		if err := a.redis.Ping(ctx).Err(); err != nil {
			a.Close()
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		limiter = ratelimit.NewRedis(a.redis, "condoguard:login:", cfg.LoginMaxAttempts, cfg.LoginLockoutWindow)
	} else {
		slog.Warn("REDIS_URL not set — login attempt limit kept in process memory (single instance only)")
		limiter = ratelimit.NewMemory(cfg.LoginMaxAttempts, cfg.LoginLockoutWindow)
	}

	a.Handler = newHandler(cfg, a.pool, limiter, logger, metrics)
	return a, nil
}

// NewRouter builds the full HTTP handler tree from environment configuration.
// It exits the process if the configured database or Redis is unreachable.
// Resources live for the whole process; use New when they must be closed.
func NewRouter(logger *slog.Logger, metrics *middleware.Metrics) http.Handler {
	cfg, err := ConfigFromEnv()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	a, err := New(context.Background(), cfg, logger, metrics)
	if err != nil {
		slog.Error("startup", "error", err)
		os.Exit(1)
	}
	return a.Handler
}

// newHandler wires all domain routes and the global middleware stack:
// CORS → RequestID → Logging → Metrics → Tenant → mux.
func newHandler(cfg Config, pool *pgxpool.Pool, limiter ratelimit.Limiter, logger *slog.Logger, metrics *middleware.Metrics) http.Handler {
	mux := http.NewServeMux()

	// ── Health ─────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", handleHealth)

	// ── Metrics (expvar) ───────────────────────────────────────────────────────
	mux.Handle("GET /metrics", expvar.Handler())

	// ── OpenAPI spec + Swagger UI ──────────────────────────────────────────────
	mux.Handle("GET /openapi.json", openapi.Handler())
	mux.Handle("GET /docs", openapi.UIHandler())

	// ── Dependencies ───────────────────────────────────────────────────────────
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		slog.Warn("JWT_SECRET_KEY not set — using insecure placeholder (not for production)")
		jwtSecret = "Y2hhbmdlLW1lLWluLXByb2R1Y3Rpb24="
	}
	jwtSvc := pkgjwt.NewService(jwtSecret)
	authMW := middleware.Authenticate(jwtSvc)

	var (
		authRepo     auth.Repository
		userRepo     user.Repository
		residentRepo resident.Repository
		shopRepo     shopowner.Repository
		expenseRepo  expense.Repository
		notifRepo    notification.Repository
	)
	if pool != nil {
		authRepo = auth.NewPostgresRepository(pool)
		userRepo = user.NewPostgresRepository(pool)
		residentRepo = resident.NewPostgresRepository(pool)
		shopRepo = shopowner.NewPostgresRepository(pool)
		expenseRepo = expense.NewPostgresRepository(pool)
		notifRepo = notification.NewPostgresRepository(pool)
	} else {
		authRepo = auth.NewInMemoryRepository()
		userRepo = user.NewInMemoryRepository()
		residentRepo = resident.NewInMemoryRepository()
		shopRepo = shopowner.NewInMemoryRepository()
		expenseRepo = expense.NewInMemoryRepository()
		notifRepo = notification.NewInMemoryRepository()
	}

	// ── Auth (public; login limited per IP + username) ─────────────────────────
	authHandler := auth.NewHandler(auth.NewService(authRepo, jwtSvc))
	mux.Handle("POST /auth/login", middleware.LoginLimit(limiter)(authHandler))
	mux.Handle("/auth/", authHandler)

	// ── Users (ROLE_ADMIN) ─────────────────────────────────────────────────────
	mux.Handle("/users", user.NewHandler(user.NewService(userRepo), authMW))
	mux.Handle("/users/", user.NewHandler(user.NewService(userRepo), authMW))

	// ── Residents ─────────────────────────────────────────────────────────────
	mux.Handle("/residents", resident.NewHandler(resident.NewService(residentRepo), authMW))
	mux.Handle("/residents/", resident.NewHandler(resident.NewService(residentRepo), authMW))

	// ── ShopOwners ────────────────────────────────────────────────────────────
	mux.Handle("/shopOwners", shopowner.NewHandler(shopowner.NewService(shopRepo), authMW))
	mux.Handle("/shopOwners/", shopowner.NewHandler(shopowner.NewService(shopRepo), authMW))

	// ── Expenses ──────────────────────────────────────────────────────────────
	mux.Handle("/expenses", expense.NewHandler(expense.NewService(expenseRepo), authMW))
	mux.Handle("/expenses/", expense.NewHandler(expense.NewService(expenseRepo), authMW))

	// ── Notifications ─────────────────────────────────────────────────────────
	mux.Handle("/notifications", notification.NewHandler(notification.NewService(notifRepo), authMW))
	mux.Handle("/notifications/", notification.NewHandler(notification.NewService(notifRepo), authMW))

	// ── Global middleware stack (outermost = first executed) ──────────────────
	// Order: CORS → RequestID → Logging → Metrics → Tenant → mux
	return middleware.CORS(
		middleware.RequestID(
			middleware.Logging(logger)(
				metrics.Middleware(
					tenant.Middleware(cfg.CondominiumID)(mux),
				),
			),
		),
	)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
