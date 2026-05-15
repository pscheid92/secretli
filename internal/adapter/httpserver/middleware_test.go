package httpserver

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v4"

	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

func TestHTTPErrorHandler(t *testing.T) {
	t.Run("converts app error to JSON response", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httpErrorHandler
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		httpErrorHandler(apperrors.BadRequestError("test error"), c)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		var resp apperrors.ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Error != "test error" {
			t.Errorf("error = %q, want %q", resp.Error, "test error")
		}
	})

	t.Run("converts unknown error to 500", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httpErrorHandler
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		httpErrorHandler(echo.NewHTTPError(http.StatusBadGateway, "bad gateway"), c)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("does not write to committed response", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httpErrorHandler
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Commit the response by writing a status
		c.NoContent(http.StatusOK)

		httpErrorHandler(apperrors.BadRequestError("should not appear"), c)

		// Status should remain 200, not be changed to 400
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d (should not change after commit)", rec.Code, http.StatusOK)
		}
	})
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

func TestMetricsAuth(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", authHeader: "Basic metrics-secret", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", authHeader: "Bearer wrong", wantStatus: http.StatusUnauthorized},
		{name: "valid token", authHeader: "Bearer metrics-secret", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.authHeader != "" {
				req.Header.Set(echo.HeaderAuthorization, tt.authHeader)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			handler := metricsAuth("metrics-secret")(func(c echo.Context) error {
				return c.NoContent(http.StatusOK)
			})

			if err := handler(c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusUnauthorized && rec.Header().Get(echo.HeaderWWWAuthenticate) == "" {
				t.Error("missing WWW-Authenticate header")
			}
		})
	}
}

func TestCORSMiddlewareAllowsRangeAPIHeaders(t *testing.T) {
	e := echo.New()
	handler := corsMiddleware([]string{"https://app.example"})(func(c echo.Context) error {
		c.Response().Header().Set("Content-Range", "bytes 0-1/10")
		return c.NoContent(http.StatusPartialContent)
	})

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/secrets/id/blob", nil)
	preflight.Header.Set(echo.HeaderOrigin, "https://app.example")
	preflight.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodGet)
	preflight.Header.Set(echo.HeaderAccessControlRequestHeaders, "Authorization, Range, X-Blob-Token, X-Request-ID")
	preflightRec := httptest.NewRecorder()
	if err := handler(e.NewContext(preflight, preflightRec)); err != nil {
		t.Fatalf("preflight: %v", err)
	}

	allowHeaders := preflightRec.Header().Get(echo.HeaderAccessControlAllowHeaders)
	for _, header := range []string{"Authorization", "Range", HeaderBlobToken, echo.HeaderXRequestID} {
		if !headerListContains(allowHeaders, header) {
			t.Fatalf("Allow-Headers %q missing %q", allowHeaders, header)
		}
	}

	uploadPreflight := httptest.NewRequest(http.MethodOptions, "/api/v1/secrets/uploads/id/parts/1", nil)
	uploadPreflight.Header.Set(echo.HeaderOrigin, "https://app.example")
	uploadPreflight.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodPut)
	uploadPreflight.Header.Set(echo.HeaderAccessControlRequestHeaders, "Authorization, Content-Type, X-Request-ID, X-Part-Offset, X-Part-Size, X-Part-SHA256")
	uploadPreflightRec := httptest.NewRecorder()
	if err := handler(e.NewContext(uploadPreflight, uploadPreflightRec)); err != nil {
		t.Fatalf("upload preflight: %v", err)
	}

	uploadAllowHeaders := uploadPreflightRec.Header().Get(echo.HeaderAccessControlAllowHeaders)
	for _, header := range []string{"Authorization", "Content-Type", echo.HeaderXRequestID, HeaderPartOffset, HeaderPartSize, HeaderPartSHA256} {
		if !headerListContains(uploadAllowHeaders, header) {
			t.Fatalf("Upload Allow-Headers %q missing %q", uploadAllowHeaders, header)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/id/blob", nil)
	req.Header.Set(echo.HeaderOrigin, "https://app.example")
	rec := httptest.NewRecorder()
	if err := handler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("request: %v", err)
	}

	exposeHeaders := rec.Header().Get(echo.HeaderAccessControlExposeHeaders)
	for _, header := range []string{echo.HeaderXRequestID, "Content-Range", "Accept-Ranges", HeaderBurnAfterRead} {
		if !headerListContains(exposeHeaders, header) {
			t.Fatalf("Expose-Headers %q missing %q", exposeHeaders, header)
		}
	}
}

func headerListContains(list, header string) bool {
	header = strings.ToLower(header)
	for _, item := range strings.Split(list, ",") {
		if strings.ToLower(strings.TrimSpace(item)) == header {
			return true
		}
	}
	return false
}

// --- spaHandler tests ---

func TestSpaHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>SPA</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}

	h := spaHandler(fs.FS(fsys))

	t.Run("serves index.html for root", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h(c); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body == "" {
			t.Error("expected non-empty body for root")
		}
	})

	t.Run("serves existing static file", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h(c); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "console.log('app')" {
			t.Errorf("body = %q, want %q", body, "console.log('app')")
		}
	})

	t.Run("falls back to index.html for unknown paths", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := h(c); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body == "" {
			t.Error("expected fallback to index.html with non-empty body")
		}
	})
}
