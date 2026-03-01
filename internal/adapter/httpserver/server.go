package httpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
)

type App struct {
	echo       *echo.Echo
	addr       string
	pool       *pgxpool.Pool
	secretRepo domain.SecretRepo
	fileStore  domain.FileStore
	cfg        config.Config
	reg        *prometheus.Registry

	SecretMetrics *metrics.SecretMetrics
}

func New(cfg config.Config, pool *pgxpool.Pool, secretRepo domain.SecretRepo, fileStore domain.FileStore, reg *prometheus.Registry) *App {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = httpErrorHandler

	e.Server.ReadTimeout = 30 * time.Second
	e.Server.WriteTimeout = 60 * time.Second
	e.Server.IdleTimeout = 120 * time.Second

	a := &App{
		echo:       e,
		addr:       fmt.Sprintf(":%s", cfg.Port),
		pool:       pool,
		secretRepo: secretRepo,
		fileStore:  fileStore,
		cfg:        cfg,
		reg:        reg,
	}

	a.SecretMetrics = a.registerRoutes()

	return a
}

func (a *App) Start() error {
	return a.echo.Start(a.addr)
}

func (a *App) Shutdown(ctx context.Context) error {
	return a.echo.Shutdown(ctx)
}
