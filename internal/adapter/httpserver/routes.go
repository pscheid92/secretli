package httpserver

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
	"github.com/pscheid92/secretli/web"
)

func registerRoutes(
	r chi.Router,
	cfg config.Config,
	pool *pgxpool.Pool,
	secretRepo domain.SecretRepo,
	fileStore domain.FileStore,
	reg *prometheus.Registry,
) *metrics.SecretMetrics {
	httpMetrics := metrics.NewHTTPMetrics(reg)
	secretMetrics := metrics.NewSecretMetrics(reg)

	// Middleware stack
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(requestIDResponseHeader)
	r.Use(httpMetrics.Middleware)
	r.Use(middleware.RequestLogger(&slogLogger{}))
	r.Use(securityHeadersMiddleware)

	if origins := parseOrigins(cfg.AllowedOrigins); len(origins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   origins,
			AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "X-Retrieval-Token", "X-Deletion-Token"},
			AllowCredentials: true,
			MaxAge:           86400,
		}))
	}

	// Metrics
	r.Handle("/metrics", metrics.Handler(reg))

	// Health (not rate limited)
	r.Get("/api/v1/health/live", Liveness)
	r.Method("GET", "/api/v1/health/ready", ReadinessWithDB(pool))

	// Secrets
	sh := NewSecretHandler(secretRepo, fileStore, cfg.MaxFileSize, secretMetrics)
	r.Route("/api/v1/secrets", func(r chi.Router) {
		// Create (10/min)
		r.Group(func(r chi.Router) {
			r.Use(rateLimitHandler(10, time.Minute))
			r.Method("POST", "/", HandlerFunc(sh.CreateSecret))
		})

		// Retrieve (30/min)
		r.Group(func(r chi.Router) {
			r.Use(rateLimitHandler(30, time.Minute))
			r.Method("POST", "/{publicID}", HandlerFunc(sh.RetrieveSecret))
			r.Method("GET", "/{publicID}/meta", HandlerFunc(sh.SecretMetadata))
		})

		// Delete (30/min)
		r.Group(func(r chi.Router) {
			r.Use(rateLimitHandler(30, time.Minute))
			r.Method("DELETE", "/{publicID}", HandlerFunc(sh.DeleteSecret))
		})
	})

	// SPA catch-all
	distFS, _ := fs.Sub(web.DistFS, "frontend/dist")
	r.NotFound(spaHandler(distFS).ServeHTTP)

	return secretMetrics
}

func rateLimitHandler(limit int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.Limit(limit, window,
		httprate.WithKeyFuncs(httprate.KeyByRealIP),
		httprate.WithLimitHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
		})),
	)
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
