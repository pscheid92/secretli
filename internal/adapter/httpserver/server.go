package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
)

type App struct {
	HTTPServer    *http.Server
	SecretMetrics *metrics.SecretMetrics
}

func New(cfg config.Config, pool *pgxpool.Pool, secretRepo domain.SecretRepo, fileStore domain.FileStore, reg *prometheus.Registry) *App {
	r := chi.NewRouter()
	secretMetrics := registerRoutes(r, cfg, pool, secretRepo, fileStore, reg)

	return &App{
		HTTPServer: &http.Server{
			Addr:         fmt.Sprintf(":%s", cfg.Port),
			Handler:      r,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		SecretMetrics: secretMetrics,
	}
}
