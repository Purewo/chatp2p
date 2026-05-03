package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chatp2p/internal/auth"
	"chatp2p/internal/config"
	"chatp2p/internal/httpapi"
	"chatp2p/internal/realtime"
	"chatp2p/internal/service"
	"chatp2p/internal/storage"
	"chatp2p/internal/store"
)

const version = "0.1.0"

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel(),
	}))

	db, err := storage.Open(storage.Config{Driver: cfg.DBDriver, DSN: cfg.DBDSN})
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := storage.Migrate(context.Background(), db, "migrations"); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	userStore := store.NewSQLiteUserStore(db)
	tokenManager := auth.NewManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)
	authService := service.NewAuthService(userStore, tokenManager)
	socialService := service.NewSocialService(authService, userStore)
	messageService := service.NewMessageService(authService, userStore)
	realtimeHub := realtime.NewHub()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(httpapi.RouterOptions{ServiceName: cfg.AppName, Version: version, Auth: authService, Social: socialService, Messages: messageService, Realtime: realtimeHub}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", cfg.HTTPAddr, "env", cfg.Environment, "dbDriver", cfg.DBDriver)
		errCh <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
}
