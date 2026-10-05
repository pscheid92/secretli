package httpserver

import (
	"io/fs"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/web"
)

// smallRequestBodyLimit bounds JSON and header-only API requests so a client
// cannot make the server buffer an arbitrarily large body before validation.
// Blob and part uploads enforce their own size limits.
const smallRequestBodyLimit = "64K"

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

	// Build version for the page footer (not rate limited)
	e.GET("/api/v1/version", VersionHandler(a.version))

	// Secrets
	sh := NewSecretHandler(a.secretRepo, a.fileStore, secretMetrics)
	secrets := e.Group("/api/v1/secrets")

	// Retrieve (30/min)
	retrieveGroup := secrets.Group("")
	retrieveGroup.Use(rateLimiter(30, time.Minute))
	retrieveGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	retrieveGroup.POST("/:publicID/retrieval-session", sh.StartRetrievalSession)
	retrieveGroup.GET("/:publicID/meta", sh.SecretMetadata)

	// Range retrieval can require many chunk requests for one authorized session.
	rangeGroup := secrets.Group("")
	rangeGroup.Use(rateLimiter(600, time.Minute))
	rangeGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	rangeGroup.GET("/:publicID/blob", sh.RetrieveSecretRange)

	// Delete (30/min)
	deleteGroup := secrets.Group("")
	deleteGroup.Use(rateLimiter(30, time.Minute))
	deleteGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	deleteGroup.DELETE("/:publicID", sh.DeleteSecret)

	// Short-code transfers. Guessing is bounded by one claim per transfer,
	// not by these limits; they only keep the relay from being flooded.
	th := NewTransferHandler(a.secretRepo, a.transferEvents)
	transfers := e.Group("/api/v1/transfers")

	transferCreateGroup := transfers.Group("")
	transferCreateGroup.Use(rateLimiter(10, time.Minute))
	transferCreateGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	transferCreateGroup.POST("", th.CreateTransfer)

	transferClaimGroup := transfers.Group("")
	transferClaimGroup.Use(rateLimiter(10, time.Minute))
	transferClaimGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	transferClaimGroup.POST("/claim", th.ClaimTransfer)

	// A waiting side long-polls for up to 25 s per request.
	transferMessageGroup := transfers.Group("")
	transferMessageGroup.Use(rateLimiter(300, time.Minute))
	transferMessageGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	transferMessageGroup.PUT("/:transferID/messages/:phase", th.PutMessage)
	transferMessageGroup.GET("/:transferID/messages/:phase", th.GetMessage)

	transferCloseGroup := transfers.Group("")
	transferCloseGroup.Use(rateLimiter(30, time.Minute))
	transferCloseGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	transferCloseGroup.DELETE("/:transferID", th.CloseTransfer)

	uh := NewUploadHandler(a.secretRepo, a.fileStore, a.cfg.MaxFileSize, secretMetrics)
	uploads := e.Group("/api/v1/secrets/uploads")

	// Starting a session is what creates a secret, so it carries the create
	// budget.
	uploadCreateGroup := uploads.Group("")
	uploadCreateGroup.Use(rateLimiter(10, time.Minute))
	uploadCreateGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	uploadCreateGroup.POST("", uh.CreateUploadSession)

	// Operations on a session that already exists are guarded by its upload
	// token and happen at least once per upload; charging them to the create
	// budget would halve the number of shares a client can make.
	uploadSessionGroup := uploads.Group("")
	uploadSessionGroup.Use(rateLimiter(60, time.Minute))
	uploadSessionGroup.Use(middleware.BodyLimit(smallRequestBodyLimit))
	uploadSessionGroup.POST("/:sessionID/complete", uh.CompleteUploadSession)
	uploadSessionGroup.DELETE("/:sessionID", uh.AbortUploadSession)

	uploadPartGroup := uploads.Group("")
	uploadPartGroup.Use(rateLimiter(600, time.Minute))
	uploadPartGroup.PUT("/:sessionID/parts/:partNumber", uh.UploadPart)

	// SPA catch-all
	distFS, _ := fs.Sub(web.DistFS, "frontend/dist")
	e.GET("/*", spaHandler(distFS))

	return secretMetrics
}
