package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/config"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
	"github.com/pscheid92/secretli/web"
)

// App holds the HTTP server and dependencies needed by the cleanup worker.
type App struct {
	HTTPServer  *http.Server
	SecretRepo  store.SecretRepo
	SessionRepo store.SessionRepo
	FileStore   storage.FileStore
	RateLimiter *IPRateLimiter
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

func New(cfg config.Config, pool *pgxpool.Pool) *App {
	mux := http.NewServeMux()

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
	userRepo := store.NewPostgresUserRepo(pool)
	sessionRepo := store.NewPostgresSessionRepo(pool, cfg.SessionMaxAge)
	userSecretRepo := store.NewPostgresUserSecretRepo(pool)
	rateLimiter := NewIPRateLimiter()

	registerRoutes(mux, pool, secretRepo, fileStore, cfg.MaxFileSize, userRepo, sessionRepo, userSecretRepo, cfg, rateLimiter)

	// SPA handler as catch-all
	distFS, _ := fs.Sub(web.DistFS, "frontend/dist")
	mux.Handle("/", spaHandler(distFS))

	handler := chain(mux,
		recoveryMiddleware,
		requestIDMiddleware,
		loggingMiddleware,
		securityHeadersMiddleware,
		corsMiddleware(parseOrigins(cfg.AllowedOrigins)),
		sessionMiddleware(sessionRepo),
	)

	return &App{
		HTTPServer: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		SecretRepo:  secretRepo,
		SessionRepo: sessionRepo,
		FileStore:   fileStore,
		RateLimiter: rateLimiter,
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
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
