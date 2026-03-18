package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/victorradael/condoguard/api/internal/auth"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)

	// Auth routes (public)
	jwtSecret := os.Getenv("JWT_SECRET_KEY")
	if jwtSecret == "" {
		jwtSecret = "Y2hhbmdlLW1lLWluLXByb2R1Y3Rpb24=" // placeholder; must be overridden via env
	}
	jwtSvc := pkgjwt.NewService(jwtSecret)

	mongoURI := os.Getenv("MONGODB_URI")
	var authRepo auth.Repository
	if mongoURI != "" {
		var err error
		dbName := os.Getenv("MONGO_DB")
		if dbName == "" {
			dbName = "condoguard"
		}
		mongoRepo, err := auth.NewMongoRepository(context.Background(), mongoURI, dbName)
		if err != nil {
			slog.Error("failed to connect to MongoDB", "error", err)
			os.Exit(1)
		}
		authRepo = mongoRepo
	} else {
		slog.Warn("MONGODB_URI not set — using in-memory repository (not for production)")
		authRepo = auth.NewInMemoryRepository()
	}

	authSvc := auth.NewService(authRepo, jwtSvc)
	authHandler := auth.NewHandler(authSvc)
	mux.Handle("/auth/", authHandler)

	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      NewRouter(),
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
