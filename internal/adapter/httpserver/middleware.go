package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requestIDResponseHeader copies chi's request ID from context to the X-Request-Id response header.
func requestIDResponseHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}

// slogLogger implements chi's middleware.LogFormatter to emit structured slog entries.
type slogLogger struct{}

func (l *slogLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	return &slogEntry{
		ctx:    r.Context(),
		method: r.Method,
		path:   r.URL.Path,
		start:  time.Now(),
	}
}

type slogEntry struct {
	ctx    context.Context
	method string
	path   string
	start  time.Time
}

func (e *slogEntry) Write(status, _ int, _ http.Header, _ time.Duration, _ interface{}) {
	slog.InfoContext(e.ctx, "request",
		"method", e.method,
		"path", e.path,
		"status", status,
		"duration_ms", time.Since(e.start).Milliseconds(),
	)
}

func (e *slogEntry) Panic(v interface{}, _ []byte) {
	slog.ErrorContext(e.ctx, "panic recovered",
		"error", v,
		"method", e.method,
		"path", e.path,
	)
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
