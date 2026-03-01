package server

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/pscheid92/secretli/internal/handler"
	"github.com/pscheid92/secretli/internal/store"
	"golang.org/x/time/rate"
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
		method:    r.Method,
		path:      r.URL.Path,
		requestID: middleware.GetReqID(r.Context()),
		start:     time.Now(),
	}
}

type slogEntry struct {
	method    string
	path      string
	requestID string
	start     time.Time
}

func (e *slogEntry) Write(status, _ int, _ http.Header, _ time.Duration, _ interface{}) {
	slog.Info("request",
		"method", e.method,
		"path", e.path,
		"status", status,
		"duration_ms", time.Since(e.start).Milliseconds(),
		"request_id", e.requestID,
	)
}

func (e *slogEntry) Panic(v interface{}, _ []byte) {
	slog.Error("panic recovered",
		"error", v,
		"method", e.method,
		"path", e.path,
	)
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func sessionMiddleware(sessionRepo store.SessionRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_id")
			if err == nil && cookie.Value != "" {
				user, err := sessionRepo.GetByIDWithUser(r.Context(), cookie.Value)
				if err == nil && user != nil {
					r = r.WithContext(handler.ContextWithUser(r.Context(), user))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPRateLimiter manages per-IP rate limiters.
type IPRateLimiter struct {
	limiters sync.Map // map[string]*limiterEntry
}

type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
	mu         sync.Mutex
}

// NewIPRateLimiter creates a new IPRateLimiter.
func NewIPRateLimiter() *IPRateLimiter {
	return &IPRateLimiter{}
}

func (rl *IPRateLimiter) getLimiter(ip string, rps float64, burst int) *rate.Limiter {
	val, ok := rl.limiters.Load(ip)
	if ok {
		entry := val.(*limiterEntry)
		entry.mu.Lock()
		entry.lastAccess = time.Now()
		entry.mu.Unlock()
		return entry.limiter
	}
	entry := &limiterEntry{
		limiter:    rate.NewLimiter(rate.Limit(rps), burst),
		lastAccess: time.Now(),
	}
	actual, _ := rl.limiters.LoadOrStore(ip, entry)
	return actual.(*limiterEntry).limiter
}

// CleanupStaleEntries removes rate limiter entries not accessed in the given duration.
func (rl *IPRateLimiter) CleanupStaleEntries(maxAge time.Duration) {
	now := time.Now()
	rl.limiters.Range(func(key, value any) bool {
		entry := value.(*limiterEntry)
		entry.mu.Lock()
		stale := now.Sub(entry.lastAccess) > maxAge
		entry.mu.Unlock()
		if stale {
			rl.limiters.Delete(key)
		}
		return true
	})
}

// RateLimit returns chi-compatible middleware that rate limits by IP at the given requests-per-minute.
func RateLimit(rl *IPRateLimiter, requestsPerMinute float64) func(http.Handler) http.Handler {
	rps := requestsPerMinute / 60.0
	burst := int(requestsPerMinute)
	if burst < 1 {
		burst = 1
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			limiter := rl.getLimiter(ip, rps, burst)
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Take the first IP in the chain
		if i := len(fwd); i > 0 {
			for j, c := range fwd {
				if c == ',' {
					return fwd[:j]
				}
			}
			return fwd
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
