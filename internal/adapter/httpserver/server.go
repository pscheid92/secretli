package httpserver

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
	"github.com/pscheid92/secretli/web"
)

// App holds the HTTP server and dependencies needed by the cleanup worker.
type App struct {
	HTTPServer    *http.Server
	SecretMetrics *metrics.SecretMetrics
}

func parseOrigins(origins string) []string {
	if origins == "" {
		return nil
	}
	var result []string
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			result = append(result, o)
		}
	}
	return result
}

func New(
	cfg config.Config,
	pool *pgxpool.Pool,
	secretRepo domain.SecretRepo,
	fileStore domain.FileStore,
	reg *prometheus.Registry,
) *App {
	r := chi.NewRouter()

	httpMetrics := metrics.NewHTTPMetrics(reg)
	secretMetrics := metrics.NewSecretMetrics(reg)

	// Global middleware
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(requestIDResponseHeader)
	r.Use(httpMetrics.Middleware)
	r.Use(middleware.RequestLogger(&slogLogger{}))
	r.Use(securityHeadersMiddleware)

	allowedOrigins := parseOrigins(cfg.AllowedOrigins)
	if len(allowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   allowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "X-Retrieval-Token", "X-Deletion-Token"},
			AllowCredentials: true,
			MaxAge:           86400,
		}))
	}

	r.Handle("/metrics", metrics.Handler(reg))
	registerRoutes(r, pool, secretRepo, fileStore, cfg.MaxFileSize, secretMetrics)

	// SPA handler as catch-all
	distFS, _ := fs.Sub(web.DistFS, "frontend/dist")
	r.NotFound(spaHandler(distFS).ServeHTTP)

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

func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}

		f, err := fsys.Open(path)
		if err != nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		_ = f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
