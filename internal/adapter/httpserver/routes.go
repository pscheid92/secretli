package httpserver

import (
	"io/fs"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/web"
)

func (a *App) registerRoutes() *metrics.SecretMetrics {
	e := a.echo
	httpMetrics := metrics.NewHTTPMetrics(a.reg)
	secretMetrics := metrics.NewSecretMetrics(a.reg)

	// Middleware stack
	e.Use(middleware.Recover())
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{TargetHeader: echo.HeaderXRequestID}))
	e.Use(correlationMiddleware())
	e.Use(httpMetrics.Middleware())
	e.Use(requestLogger())
	e.Use(securityHeaders())

	if origins := parseOrigins(a.cfg.AllowedOrigins); len(origins) > 0 {
		e.Use(corsMiddleware(origins))
	}

	// Metrics
	metricsHandler := echo.WrapHandler(metrics.Handler(a.reg))
	if a.cfg.MetricsToken != "" {
		e.GET("/metrics", metricsHandler, metricsAuth(a.cfg.MetricsToken))
	} else {
		e.GET("/metrics", metricsHandler)
	}

	// Health (not rate limited)
	e.GET("/api/v1/health/live", Liveness)
	e.GET("/api/v1/health/ready", ReadinessWithDB(a.pool))

	// Secrets
	sh := NewSecretHandler(a.secretRepo, a.fileStore, a.cfg.MaxFileSize, secretMetrics)
	secrets := e.Group("/api/v1/secrets")

	// Create (10/min)
	createGroup := secrets.Group("")
	createGroup.Use(rateLimiter(10, time.Minute))
	createGroup.POST("", sh.CreateSecret)

	// Retrieve (30/min)
	retrieveGroup := secrets.Group("")
	retrieveGroup.Use(rateLimiter(30, time.Minute))
	retrieveGroup.POST("/:publicID", sh.RetrieveSecret)
	retrieveGroup.POST("/:publicID/retrieval-session", sh.StartRetrievalSession)
	retrieveGroup.GET("/:publicID/meta", sh.SecretMetadata)

	// Range retrieval can require many chunk requests for one authorized session.
	rangeGroup := secrets.Group("")
	rangeGroup.Use(rateLimiter(600, time.Minute))
	rangeGroup.GET("/:publicID/blob", sh.RetrieveSecretRange)

	// Delete (30/min)
	deleteGroup := secrets.Group("")
	deleteGroup.Use(rateLimiter(30, time.Minute))
	deleteGroup.DELETE("/:publicID", sh.DeleteSecret)

	// Chunked v2 upload/retrieval APIs.
	secretsV2 := e.Group("/api/v2/secrets")

	uploadCreateGroup := secretsV2.Group("")
	uploadCreateGroup.Use(rateLimiter(10, time.Minute))
	uploadCreateGroup.POST("/uploads", sh.CreateUpload)

	uploadGroup := secretsV2.Group("")
	uploadGroup.Use(rateLimiter(600, time.Minute))
	uploadGroup.GET("/:publicID/upload", sh.UploadStatus)
	uploadGroup.PUT("/:publicID/chunks/:index", sh.UploadChunk)
	uploadGroup.PUT("/:publicID/manifest", sh.UploadManifest)
	uploadGroup.POST("/:publicID/complete", sh.CompleteUpload)
	uploadGroup.DELETE("/:publicID/upload", sh.CancelUpload)

	retrieveV2Group := secretsV2.Group("")
	retrieveV2Group.Use(rateLimiter(600, time.Minute))
	retrieveV2Group.POST("/:publicID/retrieval-session", sh.StartRetrievalSession)
	retrieveV2Group.GET("/:publicID/meta", sh.SecretMetadata)
	retrieveV2Group.GET("/:publicID/manifest", sh.RetrieveChunkedManifest)
	retrieveV2Group.GET("/:publicID/chunks/:index", sh.RetrieveChunkedChunk)

	// SPA catch-all
	distFS, _ := fs.Sub(web.DistFS, "frontend/dist")
	e.GET("/*", spaHandler(distFS))

	return secretMetrics
}
