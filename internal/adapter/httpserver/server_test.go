package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
)

// fullMockRepo satisfies both domain.SecretRepo and domain.UploadSessionRepo
// so the complete route table, including multipart uploads, is registered.
type fullMockRepo struct {
	*mockSecretRepo
	*uploadMockRepo
}

func (fullMockRepo) AbortExpiredUploadSessions(
	_ context.Context,
	_ time.Time,
	_ func(*domain.UploadSession, bool) error,
) (int64, error) {
	return 0, nil
}

func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	repo := fullMockRepo{mockSecretRepo: newMockRepo(), uploadMockRepo: newUploadMockRepo()}
	app, err := New(cfg, nil, repo, newUploadMockStore(), prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return app
}

func createSessionBody(t *testing.T, label string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"public_id":       testPublicID(label),
		"metadata_token":  testToken(label + " metadata"),
		"blob_token":      testToken(label + " blob"),
		"deletion_token":  testToken(label + " deletion"),
		"encrypted_meta":  testEncryptedMeta(),
		"expiration":      "1d",
		"burn_after_read": false,
		"blob_size":       1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestApp_RejectsInvalidTrustedProxies(t *testing.T) {
	repo := fullMockRepo{mockSecretRepo: newMockRepo(), uploadMockRepo: newUploadMockRepo()}
	if _, err := New(config.Config{TrustedProxies: "nope"}, nil, repo, newUploadMockStore(), prometheus.NewRegistry()); err == nil {
		t.Fatal("expected error for invalid TRUSTED_PROXIES")
	}
}

func TestApp_SpoofedForwardedForDoesNotBypassRateLimit(t *testing.T) {
	app := newTestApp(t, config.Config{MaxFileSize: 1 << 20})

	// The upload-session create route allows 10 requests per minute per client.
	var statuses []int
	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/uploads", bytes.NewReader(createSessionBody(t, fmt.Sprintf("spoof-%d", i))))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.RemoteAddr = "203.0.113.7:4321"
		// Rotate the forwarded header on every request as an attacker would.
		req.Header.Set(echo.HeaderXForwardedFor, fmt.Sprintf("198.51.100.%d", i+1))
		rec := httptest.NewRecorder()
		app.echo.ServeHTTP(rec, req)
		statuses = append(statuses, rec.Code)
	}

	for i, code := range statuses[:10] {
		if code != http.StatusCreated {
			t.Fatalf("request %d: status = %d, want %d", i, code, http.StatusCreated)
		}
	}
	for i, code := range statuses[10:] {
		if code != http.StatusTooManyRequests {
			t.Fatalf("request %d: status = %d, want %d (rate limit bypassed via X-Forwarded-For)", i+10, code, http.StatusTooManyRequests)
		}
	}
}

func TestApp_TrustedProxyKeysRateLimitOnForwardedClient(t *testing.T) {
	app := newTestApp(t, config.Config{MaxFileSize: 1 << 20, TrustedProxies: "10.0.0.0/8"})

	send := func(label, client string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/uploads", bytes.NewReader(createSessionBody(t, label)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.RemoteAddr = "10.1.2.3:4321" // the trusted proxy
		req.Header.Set(echo.HeaderXForwardedFor, client)
		rec := httptest.NewRecorder()
		app.echo.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 10; i++ {
		if code := send(fmt.Sprintf("client-a-%d", i), "198.51.100.1"); code != http.StatusCreated {
			t.Fatalf("client A request %d: status = %d, want %d", i, code, http.StatusCreated)
		}
	}
	if code := send("client-a-over", "198.51.100.1"); code != http.StatusTooManyRequests {
		t.Fatalf("client A over limit: status = %d, want %d", code, http.StatusTooManyRequests)
	}
	// A different client behind the same proxy has its own bucket.
	if code := send("client-b", "198.51.100.2"); code != http.StatusCreated {
		t.Fatalf("client B: status = %d, want %d", code, http.StatusCreated)
	}
}

func TestApp_SessionOperationsDoNotSpendTheCreateBudget(t *testing.T) {
	app := newTestApp(t, config.Config{MaxFileSize: 1 << 20})

	// Ten sessions exhaust the create budget.
	sessions := make([]struct{ id, token string }, 0, 10)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/uploads", bytes.NewReader(createSessionBody(t, fmt.Sprintf("budget-%d", i))))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		app.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d: status = %d, want %d", i, rec.Code, http.StatusCreated)
		}
		var body struct {
			SessionID   string `json:"session_id"`
			UploadToken string `json:"upload_token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode create %d: %v", i, err)
		}
		sessions = append(sessions, struct{ id, token string }{body.SessionID, body.UploadToken})
	}

	// Each of those sessions can still be aborted: that route has its own budget.
	for i, session := range sessions {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/uploads/"+session.id, nil)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+session.token)
		rec := httptest.NewRecorder()
		app.echo.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("abort %d: status = %d, want %d", i, rec.Code, http.StatusNoContent)
		}
	}
}

func TestApp_RejectsOversizedJSONBody(t *testing.T) {
	app := newTestApp(t, config.Config{MaxFileSize: 1 << 30})

	huge := `{"encrypted_meta":"` + strings.Repeat("A", 256*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/uploads", strings.NewReader(huge))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	app.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}
