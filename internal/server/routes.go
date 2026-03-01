package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/pscheid92/secretli/internal/config"
	"github.com/pscheid92/secretli/internal/handler"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
)

func registerRoutes(
	r chi.Router,
	pinger handler.Pinger,
	secretRepo store.SecretRepo,
	fileStore storage.FileStore,
	maxFileSize int64,
	userRepo store.UserRepo,
	sessionRepo store.SessionRepo,
	userSecretRepo store.UserSecretRepo,
	cfg config.Config,
	rl *IPRateLimiter,
) {
	// Health (not rate limited)
	r.Get("/api/v1/health/live", handler.Liveness)
	r.Method("GET", "/api/v1/health/ready", handler.ReadinessWithDB(pinger))

	// Auth
	ah := handler.NewAuthHandler(userRepo, sessionRepo, cfg.SessionMaxAge, cfg.CookieDomain, cfg.CookieSecure)
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(RateLimit(rl, 5)) // 5/min for auth creation
			r.Post("/register", ah.Register)
			r.Post("/login", ah.Login)
		})
		r.Post("/logout", ah.Logout)
		r.Get("/me", ah.Me)
	})

	// Secrets
	sh := handler.NewSecretHandler(secretRepo, fileStore, userSecretRepo)
	r.Route("/api/v1/secrets", func(r chi.Router) {
		// Create (10/min)
		r.Group(func(r chi.Router) {
			r.Use(RateLimit(rl, 10))
			r.Post("/", sh.CreateSecret)
			if fileStore != nil {
				fh := handler.NewFileHandler(secretRepo, fileStore, maxFileSize, userSecretRepo)
				r.Post("/file", fh.UploadFile)
			}
		})

		// Retrieve (30/min)
		r.Group(func(r chi.Router) {
			r.Use(RateLimit(rl, 30))
			r.Post("/{publicID}", sh.RetrieveSecret)
			r.Get("/{publicID}/meta", sh.SecretMetadata)
			if fileStore != nil {
				fh := handler.NewFileHandler(secretRepo, fileStore, maxFileSize, userSecretRepo)
				r.Post("/{publicID}/file", fh.DownloadFile)
			}
		})

		// Delete (30/min)
		r.Group(func(r chi.Router) {
			r.Use(RateLimit(rl, 30))
			r.Delete("/{publicID}", sh.DeleteSecret)
		})
	})

	// User
	uh := handler.NewUserHandler(userSecretRepo)
	r.Get("/api/v1/user/secrets", uh.ListSecrets)
}
