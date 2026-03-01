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
	e.GET("/metrics", echo.WrapHandler(metrics.Handler(a.reg)))

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
	retrieveGroup.GET("/:publicID/meta", sh.SecretMetadata)

	// Delete (30/min)
	deleteGroup := secrets.Group("")
	deleteGroup.Use(rateLimiter(30, time.Minute))
	deleteGroup.DELETE("/:publicID", sh.DeleteSecret)

	// SPA catch-all
	distFS, _ := fs.Sub(web.DistFS, "frontend/dist")
	e.GET("/*", spaHandler(distFS))

	return secretMetrics
}
