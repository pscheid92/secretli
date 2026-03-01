package server

import (
	"net/http"

	"github.com/pscheid92/secretli/internal/config"
	"github.com/pscheid92/secretli/internal/handler"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
)

func registerRoutes(
	mux *http.ServeMux,
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
	authLimit := RateLimit(rl, 5)       // 5/min
	createLimit := RateLimit(rl, 10)    // 10/min
	retrieveLimit := RateLimit(rl, 30)  // 30/min
	deleteLimit := RateLimit(rl, 30)    // 30/min

	// Health (not rate limited)
	mux.HandleFunc("GET /api/v1/health/live", handler.Liveness)
	mux.Handle("GET /api/v1/health/ready", handler.ReadinessWithDB(pinger))

	// Auth
	ah := handler.NewAuthHandler(userRepo, sessionRepo, cfg.SessionMaxAge, cfg.CookieDomain, cfg.CookieSecure)
	mux.HandleFunc("POST /api/v1/auth/register", authLimit(ah.Register))
	mux.HandleFunc("POST /api/v1/auth/login", authLimit(ah.Login))
	mux.HandleFunc("POST /api/v1/auth/logout", ah.Logout)
	mux.HandleFunc("GET /api/v1/auth/me", ah.Me)

	// Secrets
	sh := handler.NewSecretHandler(secretRepo, fileStore, userSecretRepo)
	mux.HandleFunc("POST /api/v1/secrets", createLimit(sh.CreateSecret))
	mux.HandleFunc("POST /api/v1/secrets/{publicID}", retrieveLimit(sh.RetrieveSecret))
	mux.HandleFunc("GET /api/v1/secrets/{publicID}/meta", retrieveLimit(sh.SecretMetadata))
	mux.HandleFunc("DELETE /api/v1/secrets/{publicID}", deleteLimit(sh.DeleteSecret))

	// File secrets
	if fileStore != nil {
		fh := handler.NewFileHandler(secretRepo, fileStore, maxFileSize, userSecretRepo)
		mux.HandleFunc("POST /api/v1/secrets/file", createLimit(fh.UploadFile))
		mux.HandleFunc("POST /api/v1/secrets/{publicID}/file", retrieveLimit(fh.DownloadFile))
	}

	// User
	uh := handler.NewUserHandler(userSecretRepo)
	mux.HandleFunc("GET /api/v1/user/secrets", uh.ListSecrets)
}
