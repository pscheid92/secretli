package cmd

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pscheid92/secretli/internal/config"
	"github.com/pscheid92/secretli/internal/server"
	"github.com/pscheid92/secretli/internal/store"
)

func Run(migrationsFS embed.FS) error {
	cfg := config.Load()

	// Handle migrate subcommand
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if cfg.DatabaseURL == "" {
			return fmt.Errorf("DATABASE_URL is required for migrations")
		}
		slog.Info("running database migrations")
		if err := store.RunMigrations(context.Background(), cfg.DatabaseURL, migrationsFS); err != nil {
			return fmt.Errorf("migrations failed: %w", err)
		}
		slog.Info("migrations complete")
		return nil
	}

	// Connect to database
	ctx := context.Background()
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer pool.Close()

	srv := server.New(cfg, pool)

	// Channel to receive server errors
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", "signal", sig.String())
	case err := <-errCh:
		slog.Error("server error", "error", err)
		return err
	}

	// Graceful shutdown with 30s timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Info("shutting down server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
		return err
	}

	slog.Info("server stopped")
	return nil
}
