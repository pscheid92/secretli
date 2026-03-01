package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/pscheid92/secretli/internal/adapter/httpserver"
	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/adapter/s3"
	"github.com/pscheid92/secretli/internal/cleanup"
	"github.com/pscheid92/secretli/internal/platform/config"
	"github.com/pscheid92/secretli/internal/platform/correlation"
)

func Run() error {
	// Set up correlated logger: injects request_id into every log record.
	baseHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(correlation.NewHandler(baseHandler)))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	pool, err := setupDatabase(cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	secretRepo := postgres.NewSecretRepo(pool)

	fileStore, err := s3.NewClient(cfg.S3)
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}

	reg := metrics.NewRegistry()
	app := httpserver.New(cfg, pool, secretRepo, fileStore, reg)

	worker := cleanup.NewWorker(
		cfg.CleanupInterval,
		secretRepo,
		fileStore,
		app.SecretMetrics,
	)

	return runGracefulShutdown(app, worker)
}

func setupDatabase(cfg config.Config) (*pgxpool.Pool, error) {
	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	slog.Info("running database migrations")
	if err := postgres.RunMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrations failed: %w", err)
	}
	slog.Info("migrations complete")

	return pool, nil
}

func runGracefulShutdown(app *httpserver.App, worker *cleanup.Worker) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		slog.Info("server starting", "port", app.HTTPServer.Addr)
		if err := app.HTTPServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return app.HTTPServer.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		worker.Run(ctx)
		return nil
	})

	err := g.Wait()
	slog.Info("server stopped")
	return err
}
