package httpserver

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"

	"github.com/pscheid92/secretli/internal/platform/correlation"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

func httpErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	appErr := apperrors.AsAppError(err)

	if appErr.Type == apperrors.Internal {
		slog.ErrorContext(c.Request().Context(), appErr.Message, "error", appErr.Cause)
	}

	_ = c.JSON(appErr.HTTPStatus(), appErr.ToResponse())
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

func correlationMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			id := c.Response().Header().Get(echo.HeaderXRequestID)
			if id != "" {
				ctx := correlation.WithRequestID(c.Request().Context(), id)
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	}
}

func requestLogger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:   true,
		LogURIPath:  true,
		LogStatus:   true,
		LogLatency:  true,
		LogError:    true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				slog.ErrorContext(c.Request().Context(), "request",
					"method", v.Method,
					"path", v.URIPath,
					"status", v.Status,
					"duration_ms", v.Latency.Milliseconds(),
					"error", v.Error,
				)
			} else {
				slog.InfoContext(c.Request().Context(), "request",
					"method", v.Method,
					"path", v.URIPath,
					"status", v.Status,
					"duration_ms", v.Latency.Milliseconds(),
				)
			}
			return nil
		},
	})
}

func securityHeaders() echo.MiddlewareFunc {
	return middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:",
		ReferrerPolicy:        "no-referrer",
	})
}

func corsMiddleware(origins []string) echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", echo.HeaderAuthorization, echo.HeaderXRequestID, "Range", HeaderMetadataToken, HeaderBlobToken, HeaderDeletionToken, HeaderPartOffset, HeaderPartSize, HeaderPartSHA256},
		ExposeHeaders:    []string{echo.HeaderXRequestID, "Accept-Ranges", "Content-Range", "Content-Length", HeaderBurnAfterRead},
		AllowCredentials: true,
		MaxAge:           86400,
	})
}

func metricsAuth(token string) echo.MiddlewareFunc {
	const prefix = "Bearer "

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth := c.Request().Header.Get(echo.HeaderAuthorization)
			if !strings.HasPrefix(auth, prefix) || !crypto.TokensEqual(strings.TrimPrefix(auth, prefix), token) {
				c.Response().Header().Set(echo.HeaderWWWAuthenticate, `Bearer realm="metrics"`)
				return c.NoContent(http.StatusUnauthorized)
			}
			return next(c)
		}
	}
}

func rateLimiter(limit int, window time.Duration) echo.MiddlewareFunc {
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(float64(limit) / window.Seconds()),
				Burst:     limit,
				ExpiresIn: 3 * time.Minute,
			},
		),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			c.Response().Header().Set("Retry-After", "60")
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		},
	})
}

func spaHandler(fsys fs.FS) echo.HandlerFunc {
	fileServer := http.FileServer(http.FS(fsys))

	return func(c echo.Context) error {
		path := strings.TrimPrefix(c.Request().URL.Path, "/")
		if path == "" {
			path = "."
		}

		f, err := fsys.Open(path)
		if err != nil {
			c.Request().URL.Path = "/"
			fileServer.ServeHTTP(c.Response(), c.Request())
			return nil
		}
		_ = f.Close()

		fileServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
