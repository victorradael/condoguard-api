package main

import (
	"context"
	"expvar"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/victorradael/condoguard/api/internal/auth"
	"github.com/victorradael/condoguard/api/internal/expense"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/notification"
	"github.com/victorradael/condoguard/api/internal/resident"
	"github.com/victorradael/condoguard/api/internal/shopowner"
	"github.com/victorradael/condoguard/api/internal/user"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

func NewRouter(logger *slog.Logger, metrics *middleware.Metrics) http.Handler {
	mux := http.NewServeMux()

	// ── Health ─────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /health", handleHealth)

	// ── Metrics (expvar) ───────────────────────────────────────────────────────
	mux.Handle("GET /metrics", expvar.Handler())

	// ── Dependencies ───────────────────────────────────────────────────────────
	jwtSecret := os.Getenv("JWT_SECRET_KEY")
	if jwtSecret == "" {
		slog.Warn("JWT_SECRET_KEY not set — using insecure placeholder (not for production)")
		jwtSecret = "Y2hhbmdlLW1lLWluLXByb2R1Y3Rpb24="
	}
	jwtSvc := pkgjwt.NewService(jwtSecret)
	authMW := middleware.Authenticate(jwtSvc)

	mongoURI := os.Getenv("MONGODB_URI")
	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "condoguard"
	}

	ctx := context.Background()

	// ── Auth (public) ──────────────────────────────────────────────────────────
	var authRepo auth.Repository
	if mongoURI != "" {
		mr, err := auth.NewMongoRepository(ctx, mongoURI, dbName)
		if err != nil {
			slog.Error("mongo connect (auth)", "error", err)
			os.Exit(1)
		}
		authRepo = mr
	} else {
		slog.Warn("MONGODB_URI not set — auth using in-memory repository")
		authRepo = auth.NewInMemoryRepository()
	}
	authSvc := auth.NewService(authRepo, jwtSvc)
	mux.Handle("/auth/", auth.NewHandler(authSvc))

	// ── Users (ROLE_ADMIN) ─────────────────────────────────────────────────────
	var userRepo user.Repository
	if mongoURI != "" {
		mr, err := user.NewMongoRepository(ctx, mongoURI, dbName)
		if err != nil {
			slog.Error("mongo connect (user)", "error", err)
			os.Exit(1)
		}
		userRepo = mr
	} else {
		userRepo = user.NewInMemoryRepository()
	}
	mux.Handle("/users", user.NewHandler(user.NewService(userRepo), authMW))
	mux.Handle("/users/", user.NewHandler(user.NewService(userRepo), authMW))

	// ── Residents ─────────────────────────────────────────────────────────────
	var residentRepo resident.Repository
	if mongoURI != "" {
		mr, err := resident.NewMongoRepository(ctx, mongoURI, dbName)
		if err != nil {
			slog.Error("mongo connect (resident)", "error", err)
			os.Exit(1)
		}
		residentRepo = mr
	} else {
		residentRepo = resident.NewInMemoryRepository()
	}
	mux.Handle("/residents", resident.NewHandler(resident.NewService(residentRepo), authMW))
	mux.Handle("/residents/", resident.NewHandler(resident.NewService(residentRepo), authMW))

	// ── ShopOwners ────────────────────────────────────────────────────────────
	var shopRepo shopowner.Repository
	if mongoURI != "" {
		mr, err := shopowner.NewMongoRepository(ctx, mongoURI, dbName)
		if err != nil {
			slog.Error("mongo connect (shopowner)", "error", err)
			os.Exit(1)
		}
		shopRepo = mr
	} else {
		shopRepo = shopowner.NewInMemoryRepository()
	}
	mux.Handle("/shopOwners", shopowner.NewHandler(shopowner.NewService(shopRepo), authMW))
	mux.Handle("/shopOwners/", shopowner.NewHandler(shopowner.NewService(shopRepo), authMW))

	// ── Expenses ──────────────────────────────────────────────────────────────
	var expenseRepo expense.Repository
	if mongoURI != "" {
		mr, err := expense.NewMongoRepository(ctx, mongoURI, dbName)
		if err != nil {
			slog.Error("mongo connect (expense)", "error", err)
			os.Exit(1)
		}
		expenseRepo = mr
	} else {
		expenseRepo = expense.NewInMemoryRepository()
	}
	mux.Handle("/expenses", expense.NewHandler(expense.NewService(expenseRepo), authMW))
	mux.Handle("/expenses/", expense.NewHandler(expense.NewService(expenseRepo), authMW))

	// ── Notifications ─────────────────────────────────────────────────────────
	var notifRepo notification.Repository
	if mongoURI != "" {
		mr, err := notification.NewMongoRepository(ctx, mongoURI, dbName)
		if err != nil {
			slog.Error("mongo connect (notification)", "error", err)
			os.Exit(1)
		}
		notifRepo = mr
	} else {
		notifRepo = notification.NewInMemoryRepository()
	}
	mux.Handle("/notifications", notification.NewHandler(notification.NewService(notifRepo), authMW))
	mux.Handle("/notifications/", notification.NewHandler(notification.NewService(notifRepo), authMW))

	// ── Global middleware stack (outermost = first executed) ──────────────────
	// Order: RequestID → Logging → Metrics → mux
	return middleware.RequestID(
		middleware.Logging(logger)(
			metrics.Middleware(mux),
		),
	)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	metrics := middleware.NewMetrics()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      NewRouter(logger, metrics),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Info("server shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
