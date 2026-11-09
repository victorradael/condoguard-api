// Package app wires all domain handlers into a single http.Handler.
// It is separated from cmd/server/main.go so tests can import it directly.
package app

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/auth"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/condominium"
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
	Production            bool           // APP_ENV=production: DATABASE_URL, REDIS_URL and CORS_ALLOWED_ORIGINS become required
	DatabaseURL           string         // DATABASE_URL; empty = in-memory repositories (dev/tests only)
	MigrationsURL         string         // DATABASE_MIGRATIONS_URL: schema owner role for the migrations (default DatabaseURL)
	MigrateOnStart        bool           // DB_MIGRATE_ON_START (default true)
	RedisURL              string         // REDIS_URL; empty = in-memory login limiter (single instance)
	CondominiumID         string         // DEFAULT_CONDOMINIUM_ID: id or name of the condominium that receives public sign-ups (default "default"); none found = sign-up closed
	LoginMaxAttempts      int            // LOGIN_MAX_ATTEMPTS per IP + e-mail (default 5)
	LoginMaxAttemptsPerIP int            // LOGIN_MAX_ATTEMPTS_PER_IP across e-mails (default 50)
	LoginLockoutWindow    time.Duration  // LOGIN_LOCKOUT_WINDOW (default 15m)
	RegisterMaxPerIP      int            // REGISTER_MAX_PER_IP sign-ups per IP and window (default 10; 0 in Config disables)
	RegisterWindow        time.Duration  // REGISTER_WINDOW (default 1h)
	JWTSecret             string         // JWT_SECRET_KEY (required): Base64 of >= 32 random bytes
	AccessTokenTTL        time.Duration  // ACCESS_TOKEN_TTL (default 15m)
	RefreshTokenTTL       time.Duration  // REFRESH_TOKEN_TTL (default 720h)
	CORSAllowedOrigins    []string       // CORS_ALLOWED_ORIGINS (default http://localhost:3000); "*" is rejected
	CookieSecure          bool           // COOKIE_SECURE (default true); false only for plain-HTTP local dev
	CookieDomain          string         // COOKIE_DOMAIN (default: API host only)
	CookiePathPrefix      string         // COOKIE_PATH_PREFIX: proxy prefix of the API as the browser sees it (e.g. /api; default none)
	MetricsToken          string         // METRICS_TOKEN; empty disables /metrics
	TrustedProxies        []netip.Prefix // TRUSTED_PROXIES: CIDRs whose X-Real-IP is the client IP (default none)
}

// ConfigFromEnv reads Config from environment variables, applying defaults.
// It fails when a required setting is missing or unsafe (e.g. a weak JWT
// secret or a CORS wildcard): the API must not start insecurely.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Production:            os.Getenv("APP_ENV") == "production",
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		MigrationsURL:         os.Getenv("DATABASE_MIGRATIONS_URL"),
		MigrateOnStart:        true,
		RedisURL:              os.Getenv("REDIS_URL"),
		CondominiumID:         os.Getenv("DEFAULT_CONDOMINIUM_ID"),
		LoginMaxAttempts:      5,
		LoginMaxAttemptsPerIP: 50,
		LoginLockoutWindow:    15 * time.Minute,
		RegisterMaxPerIP:      10,
		RegisterWindow:        time.Hour,
		JWTSecret:             os.Getenv("JWT_SECRET_KEY"),
		AccessTokenTTL:        pkgjwt.DefaultExpiry,
		RefreshTokenTTL:       auth.DefaultRefreshTTL,
		CookieSecure:          true,
		CookieDomain:          os.Getenv("COOKIE_DOMAIN"),
		CookiePathPrefix:      os.Getenv("COOKIE_PATH_PREFIX"),
		MetricsToken:          os.Getenv("METRICS_TOKEN"),
	}
	if cfg.CondominiumID == "" {
		cfg.CondominiumID = "default"
	}
	if _, err := pkgjwt.DecodeSecret(cfg.JWTSecret); err != nil {
		return cfg, fmt.Errorf("JWT_SECRET_KEY: %w", err)
	}
	if cfg.MetricsToken != "" && len(cfg.MetricsToken) < 32 {
		return cfg, fmt.Errorf("METRICS_TOKEN must have at least 32 characters (or be empty to disable /metrics)")
	}
	if p := cfg.CookiePathPrefix; p != "" && (!strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") || strings.ContainsAny(p, ";, \t")) {
		return cfg, fmt.Errorf("COOKIE_PATH_PREFIX must look like /api (leading slash, no trailing slash)")
	}
	origins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if cfg.Production {
		// Production must not fall back to in-memory state or dev defaults.
		for _, req := range []struct{ env, v string }{{"DATABASE_URL", cfg.DatabaseURL}, {"REDIS_URL", cfg.RedisURL}, {"CORS_ALLOWED_ORIGINS", origins}} {
			if req.v == "" {
				return cfg, fmt.Errorf("%s is required when APP_ENV=production", req.env)
			}
		}
	}
	if origins == "" {
		origins = "http://localhost:3000"
	}
	var err error
	if cfg.CORSAllowedOrigins, err = middleware.ParseOrigins(origins); err != nil {
		return cfg, fmt.Errorf("CORS_ALLOWED_ORIGINS: %w", err)
	}
	if cfg.TrustedProxies, err = middleware.ParseTrustedProxies(os.Getenv("TRUSTED_PROXIES")); err != nil {
		return cfg, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	for _, b := range []struct {
		env string
		dst *bool
	}{{"DB_MIGRATE_ON_START", &cfg.MigrateOnStart}, {"COOKIE_SECURE", &cfg.CookieSecure}} {
		if v := os.Getenv(b.env); v != "" {
			if *b.dst, err = strconv.ParseBool(v); err != nil {
				return cfg, fmt.Errorf("%s: %w", b.env, err)
			}
		}
	}
	for _, n := range []struct {
		env string
		dst *int
	}{{"LOGIN_MAX_ATTEMPTS", &cfg.LoginMaxAttempts}, {"LOGIN_MAX_ATTEMPTS_PER_IP", &cfg.LoginMaxAttemptsPerIP}, {"REGISTER_MAX_PER_IP", &cfg.RegisterMaxPerIP}} {
		if v := os.Getenv(n.env); v != "" {
			i, err := strconv.Atoi(v)
			if err != nil || i < 1 {
				return cfg, fmt.Errorf("%s must be a positive integer", n.env)
			}
			*n.dst = i
		}
	}
	for _, d := range []struct {
		env string
		dst *time.Duration
	}{{"LOGIN_LOCKOUT_WINDOW", &cfg.LoginLockoutWindow}, {"REGISTER_WINDOW", &cfg.RegisterWindow}, {"ACCESS_TOKEN_TTL", &cfg.AccessTokenTTL}, {"REFRESH_TOKEN_TTL", &cfg.RefreshTokenTTL}} {
		if v := os.Getenv(d.env); v != "" {
			t, err := time.ParseDuration(v)
			if err != nil || t <= 0 {
				return cfg, fmt.Errorf("%s must be a positive duration (e.g. 15m)", d.env)
			}
			*d.dst = t
		}
	}
	return cfg, nil
}

// migrateOnStart applies the pending migrations. With a migrations URL it
// connects as that role (the schema owner), so the runtime role in
// DATABASE_URL needs no DDL privilege; otherwise it reuses the runtime pool.
func migrateOnStart(ctx context.Context, pool *pgxpool.Pool, migrationsURL string) error {
	if migrationsURL == "" {
		return db.MigrateUp(ctx, pool)
	}
	owner, err := db.NewPool(ctx, migrationsURL)
	if err != nil {
		return fmt.Errorf("DATABASE_MIGRATIONS_URL: %w", err)
	}
	defer owner.Close()
	return db.MigrateUp(ctx, owner)
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

	ttl := cfg.AccessTokenTTL
	if ttl <= 0 {
		ttl = pkgjwt.DefaultExpiry
	}
	jwtSvc, err := pkgjwt.NewServiceWithExpiry(cfg.JWTSecret, ttl)
	if err != nil {
		return nil, fmt.Errorf("JWT_SECRET_KEY: %w", err)
	}
	if cfg.LoginMaxAttempts < 1 || cfg.LoginLockoutWindow <= 0 {
		return nil, fmt.Errorf("login limit: LoginMaxAttempts and LoginLockoutWindow must be positive")
	}

	if cfg.DatabaseURL != "" {
		pool, err := db.NewPool(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		a.pool = pool
		if cfg.MigrateOnStart {
			if err := migrateOnStart(ctx, pool, cfg.MigrationsURL); err != nil {
				a.Close()
				return nil, err
			}
		}
	} else {
		slog.Warn("DATABASE_URL not set — using in-memory repositories (not for production)")
	}

	var limiter, ipLimiter, registerLimiter ratelimit.Limiter
	registerOn := cfg.RegisterMaxPerIP > 0 && cfg.RegisterWindow > 0
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			// The parse error quotes the URL, password included: keep it out of the log.
			a.Close()
			return nil, errors.New("REDIS_URL: invalid URL")
		}
		a.redis = redis.NewClient(opts)
		if err := a.redis.Ping(ctx).Err(); err != nil {
			a.Close()
			return nil, fmt.Errorf("redis ping: %w", err)
		}
		limiter = ratelimit.NewRedis(a.redis, "condoguard:login:", cfg.LoginMaxAttempts, cfg.LoginLockoutWindow)
		if cfg.LoginMaxAttemptsPerIP > 0 {
			ipLimiter = ratelimit.NewRedis(a.redis, "condoguard:login-ip:", cfg.LoginMaxAttemptsPerIP, cfg.LoginLockoutWindow)
		}
		if registerOn {
			registerLimiter = ratelimit.NewRedis(a.redis, "condoguard:register-ip:", cfg.RegisterMaxPerIP, cfg.RegisterWindow)
		}
	} else {
		slog.Warn("REDIS_URL not set — login attempt limit kept in process memory (single instance only)")
		limiter = ratelimit.NewMemory(cfg.LoginMaxAttempts, cfg.LoginLockoutWindow)
		if cfg.LoginMaxAttemptsPerIP > 0 {
			ipLimiter = ratelimit.NewMemory(cfg.LoginMaxAttemptsPerIP, cfg.LoginLockoutWindow)
		}
		if registerOn {
			registerLimiter = ratelimit.NewMemory(cfg.RegisterMaxPerIP, cfg.RegisterWindow)
		}
	}

	if a.pool != nil {
		// The default condominium may be given by name; the database knows its id.
		id, found, err := condominium.NewPostgresRepository(a.pool).Lookup(ctx, cfg.CondominiumID)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("DEFAULT_CONDOMINIUM_ID: %w", err)
		}
		cfg.CondominiumID = id
		if !found {
			slog.Warn("DEFAULT_CONDOMINIUM_ID matches no condominium — public sign-up is closed")
		}
	}

	a.Handler = newHandler(cfg, jwtSvc, a.pool, limiters{login: limiter, loginIP: ipLimiter, register: registerLimiter}, logger, metrics)
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
// SecurityHeaders → CORS → RequestID → Logging → Metrics → MaxBody → ClientIP → Tenant → mux.
// limiters are the request limiters of the public auth routes; nil = off.
type limiters struct {
	login, loginIP, register ratelimit.Limiter
}

func newHandler(cfg Config, jwtSvc *pkgjwt.Service, pool *pgxpool.Pool, lim limiters, logger *slog.Logger, metrics *middleware.Metrics) http.Handler {
	mux := http.NewServeMux()
	authMW := middleware.Authenticate(jwtSvc, cfg.CORSAllowedOrigins...)
	adminOnly := func(h http.Handler) http.Handler {
		return authMW(authz.RequirePermission(authz.PermUsersManage)(h))
	}

	// ── Health (public) ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", handleHealth)

	// ── Metrics (expvar): METRICS_TOKEN bearer; disabled when unset ───────────
	mux.Handle("GET /metrics", middleware.StaticToken(cfg.MetricsToken)(expvar.Handler()))

	// ── OpenAPI spec + Swagger UI (admins) ─────────────────────────────────────
	mux.Handle("GET /openapi.json", adminOnly(openapi.Handler()))
	mux.Handle("GET /docs", adminOnly(openapi.UIHandler()))

	// ── Dependencies ───────────────────────────────────────────────────────────
	var (
		authRepo     auth.Repository
		tokenStore   auth.TokenStore
		userRepo     user.Repository
		residentRepo resident.Repository
		shopRepo     shopowner.Repository
		expenseRepo  expense.Repository
		notifRepo    notification.Repository
		condoRepo    condominium.Repository
		auditStore   audit.Store
		trail        audit.Trail
	)
	if pool != nil {
		authRepo = auth.NewPostgresRepository(pool)
		tokenStore = auth.NewPostgresTokenStore(pool)
		userRepo = user.NewPostgresRepository(pool)
		residentRepo = resident.NewPostgresRepository(pool)
		shopRepo = shopowner.NewPostgresRepository(pool)
		expenseRepo = expense.NewPostgresRepository(pool)
		notifRepo = notification.NewPostgresRepository(pool)
		condoRepo = condominium.NewPostgresRepository(pool)
		auditStore = audit.NewPostgresStore(pool)
		trail = audit.Trail{Rec: auditStore, Tx: db.PoolTx{Pool: pool}}
	} else {
		authRepo = auth.NewInMemoryRepository()
		tokenStore = auth.NewInMemoryTokenStore()
		userRepo = user.NewInMemoryRepository()
		residentRepo = resident.NewInMemoryRepository()
		shopRepo = shopowner.NewInMemoryRepository()
		expenseRepo = expense.NewInMemoryRepository()
		notifRepo = notification.NewInMemoryRepository()
		condoRepo = condominium.NewInMemoryRepository()
		auditStore = audit.NewInMemoryStore()
		trail = audit.Trail{Rec: auditStore, Tx: db.NoTx{}}
	}

	residentSvc := resident.NewService(residentRepo).WithAudit(trail)
	shopSvc := shopowner.NewService(shopRepo).WithAudit(trail)
	units := authz.UnitResolverFunc(func(ctx context.Context, userID string) (authz.Units, error) {
		rids, err := residentSvc.UnitIDsOwnedBy(ctx, userID)
		if err != nil {
			return authz.Units{}, err
		}
		sids, err := shopSvc.UnitIDsOwnedBy(ctx, userID)
		if err != nil {
			return authz.Units{}, err
		}
		return authz.Units{ResidentIDs: rids, ShopOwnerIDs: sids}, nil
	})

	// ── Auth (public; login limited per IP + e-mail and per IP, sign-up per IP) ─
	authHandler := auth.NewHandler(
		auth.NewService(authRepo, tokenStore, jwtSvc, cfg.RefreshTokenTTL),
		auth.CookieOptions{Secure: cfg.CookieSecure, Domain: cfg.CookieDomain, PathPrefix: cfg.CookiePathPrefix, AllowedOrigins: cfg.CORSAllowedOrigins},
	)
	mux.Handle("POST /auth/login", middleware.LoginLimit(lim.login, lim.loginIP)(authHandler))
	mux.Handle("POST /auth/register", middleware.IPLimit(lim.register, "too many sign-ups, try again later")(authHandler))
	mux.Handle("/auth/", authHandler)

	// ── Users (/users/me: any user; the rest: sindico) ──────────────────────
	userHandler := user.NewHandler(user.NewService(userRepo).WithAudit(trail), authMW)
	mux.Handle("/users", userHandler)
	mux.Handle("/users/", userHandler)

	// ── Condominium (current) and audit trail ──────────────────────────────────
	mux.Handle("/condominium", condominium.NewHandler(condoRepo, authMW))
	mux.Handle("/audit-log", audit.NewHandler(auditStore, authMW))

	// ── Residents (units) ──────────────────────────────────────────────────────
	residentHandler := resident.NewHandler(residentSvc, authMW)
	mux.Handle("/residents", residentHandler)
	mux.Handle("/residents/", residentHandler)

	// ── ShopOwners (/shop-owners; /shopOwners kept as an alias) ───────────────
	shopHandler := shopowner.NewHandler(shopSvc, authMW)
	for _, p := range shopowner.Prefixes {
		mux.Handle(p, shopHandler)
		mux.Handle(p+"/", shopHandler)
	}

	// ── Expenses ──────────────────────────────────────────────────────────────
	expenseHandler := expense.NewHandler(expense.NewService(expenseRepo, units).WithAudit(trail), authMW)
	mux.Handle("/expenses", expenseHandler)
	mux.Handle("/expenses/", expenseHandler)

	// ── Notifications ─────────────────────────────────────────────────────────
	notifHandler := notification.NewHandler(notification.NewService(notifRepo, units).WithAudit(trail), authMW)
	mux.Handle("/notifications", notifHandler)
	mux.Handle("/notifications/", notifHandler)

	// ── Global middleware stack (outermost = first executed) ──────────────────
	return middleware.SecurityHeaders(
		middleware.CORS(cfg.CORSAllowedOrigins)(
			middleware.RequestID(
				middleware.Logging(logger)(
					metrics.Middleware(
						middleware.MaxBody(middleware.DefaultMaxBodyBytes)(
							middleware.ClientIP(cfg.TrustedProxies)(
								middleware.WithRequestMeta(
									tenant.Middleware(cfg.CondominiumID)(mux),
								),
							),
						),
					),
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
