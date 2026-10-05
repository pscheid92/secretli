package httpserver

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
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

	// Errors raised by Echo itself (body limit, router, method not allowed)
	// carry their own status; do not collapse them into 500s.
	if httpErr, ok := errors.AsType[*echo.HTTPError](err); ok {
		status := httpErr.Code
		message := http.StatusText(status)
		if status >= http.StatusInternalServerError {
			slog.ErrorContext(c.Request().Context(), "unhandled http error", "status", status, "error", httpErr)
			message = "internal server error"
		} else if text, ok := httpErr.Message.(string); ok && text != "" {
			message = text
		}
		_ = c.JSON(status, apperrors.ErrorResponse{Error: message})
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
			attrs := []any{
				"method", v.Method,
				"path", v.URIPath,
				"status", v.Status,
				"duration_ms", v.Latency.Milliseconds(),
			}
			if v.Error != nil {
				attrs = append(attrs, "error", v.Error)
			}
			slog.Log(c.Request().Context(), requestLogLevel(v.Status), "request", attrs...)
			return nil
		},
	})
}

// requestLogLevel keeps ERROR for server faults. A 4xx is a client mistake or
// an expected outcome, such as a transfer the other side ended or a claim
// already taken, so it is logged at INFO together with its error.
func requestLogLevel(status int) slog.Level {
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}
	return slog.LevelInfo
}

// permissionsPolicy lets the QR scanner use the camera on this origin only and
// turns off the other powerful features Secretli never needs.
const permissionsPolicy = "camera=(self), microphone=(), geolocation=()"

func securityHeaders() echo.MiddlewareFunc {
	secure := middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:",
		ReferrerPolicy:        "no-referrer",
	})

	// Echo's SecureConfig has no Permissions-Policy field.
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		handler := secure(next)
		return func(c echo.Context) error {
			c.Response().Header().Set("Permissions-Policy", permissionsPolicy)
			return handler(c)
		}
	}
}

func corsMiddleware(origins []string) echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", echo.HeaderAuthorization, echo.HeaderXRequestID, "Range", HeaderMetadataToken, HeaderBlobToken, HeaderDeletionToken, HeaderPartOffset, HeaderPartSize, HeaderPartSHA256},
		ExposeHeaders:    []string{echo.HeaderXRequestID, "Accept-Ranges", "Content-Range", "Content-Length"},
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

// newIPExtractor returns the client IP resolver used by the rate limiters.
// With no trusted proxies the TCP peer address is used and forwarding headers
// are ignored, so clients cannot spoof their identity. When proxy CIDRs are
// configured, X-Forwarded-For is walked from the right, skipping trusted hops.
func newIPExtractor(trustedProxies string) (echo.IPExtractor, error) {
	ranges, err := parseTrustedProxies(trustedProxies)
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return echo.ExtractIPDirect(), nil
	}
	options := make([]echo.TrustOption, 0, len(ranges)+3)
	options = append(options, echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false))
	for _, r := range ranges {
		options = append(options, echo.TrustIPRange(r))
	}
	return echo.ExtractIPFromXFFHeader(options...), nil
}

func parseTrustedProxies(value string) ([]*net.IPNet, error) {
	var ranges []*net.IPNet
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if ip := net.ParseIP(entry); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				entry = fmt.Sprintf("%s/%d", ip, bits)
			}
		}
		_, ipNet, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", entry, err)
		}
		ranges = append(ranges, ipNet)
	}
	return ranges, nil
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
