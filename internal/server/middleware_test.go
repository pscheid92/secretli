package server

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pscheid92/secretli/internal/handler"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("returns 500 on panic", func(t *testing.T) {
		handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		}))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", rr.Code)
		}
	})

	t.Run("passes through on no panic", func(t *testing.T) {
		handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})
}

func TestRequestIDMiddleware(t *testing.T) {
	t.Run("generates ID when none provided", func(t *testing.T) {
		handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		id := rr.Header().Get("X-Request-ID")
		if id == "" {
			t.Error("expected X-Request-ID to be set")
		}
	})

	t.Run("preserves existing ID", func(t *testing.T) {
		handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "existing-id")
		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get("X-Request-ID"); got != "existing-id" {
			t.Errorf("expected 'existing-id', got %q", got)
		}
	})
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'",
		"Referrer-Policy":        "no-referrer",
	}
	for header, want := range expected {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("header %s: expected %q, got %q", header, want, got)
		}
	}
}

func TestCorsMiddleware(t *testing.T) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("sets CORS headers for allowed origin", func(t *testing.T) {
		handler := corsMiddleware([]string{"https://example.com"})(noop)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("expected Allow-Origin 'https://example.com', got %q", got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("expected Allow-Credentials 'true', got %q", got)
		}
	})

	t.Run("OPTIONS to allowed origin returns 204 with CORS headers", func(t *testing.T) {
		handler := corsMiddleware([]string{"https://example.com"})(noop)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("expected Allow-Origin set, got %q", got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
			t.Error("expected Access-Control-Allow-Methods to be set")
		}
	})

	t.Run("OPTIONS to disallowed origin returns 204 without CORS Allow-Origin", func(t *testing.T) {
		handler := corsMiddleware([]string{"https://example.com"})(noop)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://evil.com")
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no Allow-Origin header, got %q", got)
		}
	})

	t.Run("empty allowed origins skips CORS entirely", func(t *testing.T) {
		handler := corsMiddleware(nil)(noop)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("expected no CORS headers, got Allow-Origin %q", got)
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})
}

func TestRateLimit(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		rl := NewIPRateLimiter()
		handler := RateLimit(rl, 60)(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("blocks requests over limit with 429", func(t *testing.T) {
		rl := NewIPRateLimiter()
		// 1 request per minute with burst of 1
		handler := RateLimit(rl, 1)(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// First request should succeed (uses the burst token)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("first request: expected status 200, got %d", rr.Code)
		}

		// Second request should be rate limited
		rr2 := httptest.NewRecorder()
		handler.ServeHTTP(rr2, req)

		if rr2.Code != http.StatusTooManyRequests {
			t.Errorf("second request: expected status 429, got %d", rr2.Code)
		}
		if got := rr2.Header().Get("Retry-After"); got != "60" {
			t.Errorf("expected Retry-After '60', got %q", got)
		}
	})
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		forwarded  string
		remoteAddr string
		want       string
	}{
		{
			name:       "uses X-Forwarded-For first IP",
			forwarded:  "10.0.0.1, 10.0.0.2",
			remoteAddr: "192.168.1.1:9999",
			want:       "10.0.0.1",
		},
		{
			name:       "uses single X-Forwarded-For",
			forwarded:  "10.0.0.1",
			remoteAddr: "192.168.1.1:9999",
			want:       "10.0.0.1",
		},
		{
			name:       "falls back to RemoteAddr",
			forwarded:  "",
			remoteAddr: "192.168.1.1:9999",
			want:       "192.168.1.1",
		},
		{
			name:       "RemoteAddr without port",
			forwarded:  "",
			remoteAddr: "192.168.1.1",
			want:       "192.168.1.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := clientIP(req); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanupStaleEntries(t *testing.T) {
	rl := NewIPRateLimiter()

	// Add entries via getLimiter
	rl.getLimiter("1.1.1.1", 1, 1)
	rl.getLimiter("2.2.2.2", 1, 1)

	// Verify both entries exist
	count := 0
	rl.limiters.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 2 {
		t.Fatalf("expected 2 entries, got %d", count)
	}

	// Cleanup with zero maxAge should remove all entries (they're all "old" relative to now)
	rl.CleanupStaleEntries(0)

	count = 0
	rl.limiters.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", count)
	}
}

func TestCleanupStaleEntries_KeepsRecent(t *testing.T) {
	rl := NewIPRateLimiter()

	// Add an entry that will be recent
	rl.getLimiter("1.1.1.1", 1, 1)

	// Cleanup with a large maxAge should keep the entry
	rl.CleanupStaleEntries(time.Hour)

	count := 0
	rl.limiters.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("expected 1 entry to be kept, got %d", count)
	}
}

// --- parseOrigins tests ---

func TestParseOrigins(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string returns nil", "", nil},
		{"single origin", "https://example.com", []string{"https://example.com"}},
		{"multiple origins", "https://a.com, https://b.com", []string{"https://a.com", "https://b.com"}},
		{"trims whitespace", "  https://a.com , https://b.com  ", []string{"https://a.com", "https://b.com"}},
		{"filters empty entries", "https://a.com,,https://b.com", []string{"https://a.com", "https://b.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOrigins(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseOrigins(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseOrigins(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseOrigins(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- chain tests ---

func TestChain(t *testing.T) {
	var order []string

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw1-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw1-after")
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "mw2-before")
			next.ServeHTTP(w, r)
			order = append(order, "mw2-after")
		})
	}

	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), mw1, mw2)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("chain order = %v, want %v", order, expected)
	}
	for i := range order {
		if order[i] != expected[i] {
			t.Errorf("chain order[%d] = %q, want %q", i, order[i], expected[i])
		}
	}
}

// --- loggingMiddleware tests ---

func TestLoggingMiddleware(t *testing.T) {
	t.Run("captures status code from handler", func(t *testing.T) {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		h := loggingMiddleware(inner)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

		if rr.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("defaults to 200 when handler writes body without explicit status", func(t *testing.T) {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})
		h := loggingMiddleware(inner)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rr, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)

	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
	}
	if rr.Code != http.StatusCreated {
		t.Errorf("underlying recorder Code = %d, want %d", rr.Code, http.StatusCreated)
	}
}

// --- sessionMiddleware tests ---

type stubSessionRepo struct {
	user *model.User
	err  error
}

func (s *stubSessionRepo) Create(_ context.Context, _ int64) (string, error)       { return "", nil }
func (s *stubSessionRepo) Delete(_ context.Context, _ string) error                 { return nil }
func (s *stubSessionRepo) DeleteExpiredSessions(_ context.Context) (int64, error)   { return 0, nil }
func (s *stubSessionRepo) GetByIDWithUser(_ context.Context, _ string) (*model.User, error) {
	return s.user, s.err
}

func TestSessionMiddleware(t *testing.T) {
	t.Run("no cookie passes through without user", func(t *testing.T) {
		repo := &stubSessionRepo{}
		mw := sessionMiddleware(repo)
		var gotUser *model.User

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser = handler.UserFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})

		rr := httptest.NewRecorder()
		mw(inner).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if gotUser != nil {
			t.Error("expected no user in context without cookie")
		}
	})

	t.Run("valid session cookie injects user into context", func(t *testing.T) {
		user := &model.User{ID: 1, Email: "test@example.com"}
		repo := &stubSessionRepo{user: user}
		mw := sessionMiddleware(repo)
		var gotUser *model.User

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser = handler.UserFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
		rr := httptest.NewRecorder()
		mw(inner).ServeHTTP(rr, req)

		if gotUser == nil {
			t.Fatal("expected user in context")
		}
		if gotUser.Email != "test@example.com" {
			t.Errorf("email = %q, want %q", gotUser.Email, "test@example.com")
		}
	})

	t.Run("invalid session cookie passes through without user", func(t *testing.T) {
		repo := &stubSessionRepo{err: store.ErrNotFound}
		mw := sessionMiddleware(repo)
		var gotUser *model.User

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser = handler.UserFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: "invalid-session"})
		rr := httptest.NewRecorder()
		mw(inner).ServeHTTP(rr, req)

		if gotUser != nil {
			t.Error("expected no user in context for invalid session")
		}
	})

	t.Run("empty cookie value passes through without user", func(t *testing.T) {
		repo := &stubSessionRepo{}
		mw := sessionMiddleware(repo)
		var gotUser *model.User

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUser = handler.UserFromContext(r.Context())
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: ""})
		rr := httptest.NewRecorder()
		mw(inner).ServeHTTP(rr, req)

		if gotUser != nil {
			t.Error("expected no user in context for empty cookie")
		}
	})
}

// --- spaHandler tests ---

func TestSpaHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>SPA</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}

	h := spaHandler(fs.FS(fsys))

	t.Run("serves index.html for root", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if body := rr.Body.String(); body == "" {
			t.Error("expected non-empty body for root")
		}
	})

	t.Run("serves existing static file", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if body := rr.Body.String(); body != "console.log('app')" {
			t.Errorf("body = %q, want %q", body, "console.log('app')")
		}
	})

	t.Run("falls back to index.html for unknown paths", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))

		if rr.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if body := rr.Body.String(); body == "" {
			t.Error("expected fallback to index.html with non-empty body")
		}
	})
}
