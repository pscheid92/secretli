package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

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

func registerRoutes(
	r chi.Router,
	pinger Pinger,
	secretRepo domain.SecretRepo,
	fileStore domain.FileStore,
	maxFileSize int64,
	secretMetrics *metrics.SecretMetrics,
) {
	// Health (not rate limited)
	r.Get("/api/v1/health/live", Liveness)
	r.Method("GET", "/api/v1/health/ready", ReadinessWithDB(pinger))

	// Secrets
	sh := NewSecretHandler(secretRepo, fileStore, maxFileSize, secretMetrics)
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
}
