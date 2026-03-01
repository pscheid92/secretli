package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pscheid92/secretli/internal/config"
	"github.com/pscheid92/secretli/internal/metrics"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
	"github.com/pscheid92/secretli/web"
)

// App holds the HTTP server and dependencies needed by the cleanup worker.
type App struct {
	HTTPServer    *http.Server
	SecretRepo    store.SecretRepo
	FileStore     storage.FileStore
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

func New(cfg config.Config, pool *pgxpool.Pool, reg *prometheus.Registry) *App {
	r := chi.NewRouter()

	httpMetrics := metrics.NewHTTPMetrics(reg)
	secretMetrics := metrics.NewSecretMetrics(reg)

	// Create S3 file store if configured
	var fileStore storage.FileStore
	if cfg.S3Endpoint != "" {
		s3Client, err := storage.NewS3Client(
			cfg.S3Endpoint, cfg.S3Bucket,
			cfg.S3AccessKey, cfg.S3SecretKey,
			cfg.S3Region, cfg.S3UseSSL,
		)
		if err != nil {
			slog.Error("failed to create S3 client, file uploads disabled", "error", err)
		} else {
			fileStore = s3Client
		}
	}

	secretRepo := store.NewPostgresSecretRepo(pool)

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
			Addr:         ":" + cfg.Port,
			Handler:      r,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		SecretRepo:    secretRepo,
		FileStore:     fileStore,
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
