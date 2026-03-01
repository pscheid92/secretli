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
) {
	// Health
	mux.HandleFunc("GET /api/v1/health/live", handler.Liveness)
	mux.Handle("GET /api/v1/health/ready", handler.ReadinessWithDB(pinger))

	// Auth
	ah := handler.NewAuthHandler(userRepo, sessionRepo, cfg.SessionMaxAge, cfg.CookieDomain, cfg.CookieSecure)
	mux.HandleFunc("POST /api/v1/auth/register", ah.Register)
	mux.HandleFunc("POST /api/v1/auth/login", ah.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", ah.Logout)
	mux.HandleFunc("GET /api/v1/auth/me", ah.Me)

	// Secrets
	sh := handler.NewSecretHandler(secretRepo, fileStore, userSecretRepo)
	mux.HandleFunc("POST /api/v1/secrets", sh.CreateSecret)
	mux.HandleFunc("POST /api/v1/secrets/{publicID}", sh.RetrieveSecret)
	mux.HandleFunc("DELETE /api/v1/secrets/{publicID}", sh.DeleteSecret)

	// File secrets
	if fileStore != nil {
		fh := handler.NewFileHandler(secretRepo, fileStore, maxFileSize, userSecretRepo)
		mux.HandleFunc("POST /api/v1/secrets/file", fh.UploadFile)
		mux.HandleFunc("POST /api/v1/secrets/{publicID}/file", fh.DownloadFile)
	}

	// User
	uh := handler.NewUserHandler(userSecretRepo)
	mux.HandleFunc("GET /api/v1/user/secrets", uh.ListSecrets)
}
