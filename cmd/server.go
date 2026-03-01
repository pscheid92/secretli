package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pscheid92/secretli/internal/cleanup"
	"github.com/pscheid92/secretli/internal/config"
	"github.com/pscheid92/secretli/internal/server"
	"github.com/pscheid92/secretli/internal/store"
)

func Run(migrationsFS fs.FS) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Connect to database
	ctx := context.Background()
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	// Run migrations automatically on startup (advisory-locked for safety)
	slog.Info("running database migrations")
	if err := store.RunMigrations(ctx, cfg.DatabaseURL, migrationsFS); err != nil {
		return fmt.Errorf("migrations failed: %w", err)
	}
	slog.Info("migrations complete")

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer pool.Close()

	app := server.New(cfg, pool)

	// Create cleanup worker
	worker := cleanup.NewWorker(
		cfg.CleanupInterval,
		app.SecretRepo,
		app.SessionRepo,
		app.FileStore,
		app.RateLimiter,
	)

	// Context for shutdown coordination
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	var wg sync.WaitGroup

	// Start cleanup worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run(shutdownCtx)
	}()

	// Start HTTP server
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := app.HTTPServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
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

	// Stop cleanup worker
	shutdownCancel()

	// Graceful HTTP shutdown with 30s timeout
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer httpCancel()

	slog.Info("shutting down server")
	if err := app.HTTPServer.Shutdown(httpCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
		return err
	}

	// Wait for cleanup worker to finish
	wg.Wait()

	slog.Info("server stopped")
	return nil
}
