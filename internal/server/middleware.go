package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pscheid92/secretli/internal/handler"
	"github.com/pscheid92/secretli/internal/store"
	"golang.org/x/time/rate"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", w.Header().Get("X-Request-ID"),
		)
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

// RateLimit returns middleware that rate limits by IP at the given requests-per-minute.
func RateLimit(rl *IPRateLimiter, requestsPerMinute float64) func(http.HandlerFunc) http.HandlerFunc {
	rps := requestsPerMinute / 60.0
	burst := int(requestsPerMinute)
	if burst < 1 {
		burst = 1
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			limiter := rl.getLimiter(ip, rps, burst)
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
				return
			}
			next(w, r)
		}
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

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowed) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")

				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Retrieval-Token, X-Deletion-Token")
					w.Header().Set("Access-Control-Max-Age", "86400")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			} else if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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

func chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
