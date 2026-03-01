package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/pscheid92/secretli/internal/handler"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
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
	pinger handler.Pinger,
	secretRepo store.SecretRepo,
	fileStore storage.FileStore,
	maxFileSize int64,
) {
	// Health (not rate limited)
	r.Get("/api/v1/health/live", handler.Liveness)
	r.Method("GET", "/api/v1/health/ready", handler.ReadinessWithDB(pinger))

	// Secrets
	sh := handler.NewSecretHandler(secretRepo, fileStore)
	r.Route("/api/v1/secrets", func(r chi.Router) {
		// Create (10/min)
		r.Group(func(r chi.Router) {
			r.Use(rateLimitHandler(10, time.Minute))
			r.Post("/", sh.CreateSecret)
			if fileStore != nil {
				fh := handler.NewFileHandler(secretRepo, fileStore, maxFileSize)
				r.Post("/file", fh.UploadFile)
			}
		})

		// Retrieve (30/min)
		r.Group(func(r chi.Router) {
			r.Use(rateLimitHandler(30, time.Minute))
			r.Post("/{publicID}", sh.RetrieveSecret)
			r.Get("/{publicID}/meta", sh.SecretMetadata)
			if fileStore != nil {
				fh := handler.NewFileHandler(secretRepo, fileStore, maxFileSize)
				r.Post("/{publicID}/file", fh.DownloadFile)
			}
		})

		// Delete (30/min)
		r.Group(func(r chi.Router) {
			r.Use(rateLimitHandler(30, time.Minute))
			r.Delete("/{publicID}", sh.DeleteSecret)
		})
	})
}
