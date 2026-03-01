package httpserver

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestRecoverer(t *testing.T) {
	t.Run("returns 500 on panic", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(middleware.Recoverer)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", rr.Code)
		}
	})

	t.Run("passes through on no panic", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(middleware.Recoverer)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})
}

func TestRequestID(t *testing.T) {
	t.Run("generates ID when none provided", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(middleware.RequestID)
		r.Use(requestIDResponseHeader)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		id := rr.Header().Get("X-Request-Id")
		if id == "" {
			t.Error("expected X-Request-Id to be set")
		}
	})

	t.Run("preserves existing ID", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(middleware.RequestID)
		r.Use(requestIDResponseHeader)
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-Id", "existing-id")
		r.ServeHTTP(rr, req)

		if got := rr.Header().Get("X-Request-Id"); got != "existing-id" {
			t.Errorf("expected 'existing-id', got %q", got)
		}
	})
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	h := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	expected := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":        "DENY",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:",
		"Referrer-Policy":         "no-referrer",
	}
	for header, want := range expected {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("header %s: expected %q, got %q", header, want, got)
		}
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

// --- slog logger tests ---

func TestSlogLogger(t *testing.T) {
	logger := &slogLogger{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	entry := logger.NewLogEntry(req)

	// Should not panic
	entry.Write(200, 0, nil, 0, nil)
	entry.Panic("test", nil)
}

// --- spaHandler tests ---

func TestSpaHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>SPA</html>")},
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
