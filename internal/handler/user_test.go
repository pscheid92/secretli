package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

// errorUserSecretRepo always returns errors for ListByUser.
type errorUserSecretRepo struct{}

func (e *errorUserSecretRepo) LinkSecret(_ context.Context, _, _ int64, _ string) error {
	return errors.New("db error")
}

func (e *errorUserSecretRepo) ListByUser(_ context.Context, _ int64, _, _ int) ([]store.SecretSummary, int64, error) {
	return nil, 0, errors.New("database connection lost")
}

// --- Mock UserSecretRepo ---

type mockUserSecretRepo struct {
	links   map[int64][]store.SecretSummary // userID -> secrets
	linkErr error
}

func newMockUserSecretRepo() *mockUserSecretRepo {
	return &mockUserSecretRepo{links: make(map[int64][]store.SecretSummary)}
}

func (m *mockUserSecretRepo) LinkSecret(_ context.Context, userID, secretID int64, label string) error {
	if m.linkErr != nil {
		return m.linkErr
	}
	m.links[userID] = append(m.links[userID], store.SecretSummary{
		PublicID:    "pub-" + label,
		Label:       label,
		SecretType:  "text",
		ExpiresAt:   time.Now().Add(time.Hour),
		CreatedAt:   time.Now(),
	})
	return nil
}

func (m *mockUserSecretRepo) ListByUser(_ context.Context, userID int64, page, perPage int) ([]store.SecretSummary, int64, error) {
	secrets := m.links[userID]
	total := int64(len(secrets))

	start := (page - 1) * perPage
	if start >= len(secrets) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(secrets) {
		end = len(secrets)
	}
	return secrets[start:end], total, nil
}

// --- Tests ---

func TestListSecrets_Success(t *testing.T) {
	usr := newMockUserSecretRepo()
	usr.links[1] = []store.SecretSummary{
		{PublicID: "pub1", Label: "first", SecretType: "text", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()},
		{PublicID: "pub2", Label: "second", SecretType: "file", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()},
	}

	h := NewUserHandler(usr)
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ListSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	secrets := resp["secrets"].([]any)
	if len(secrets) != 2 {
		t.Errorf("got %d secrets, want 2", len(secrets))
	}
	if resp["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", resp["total"])
	}
}

func TestListSecrets_Unauthenticated(t *testing.T) {
	usr := newMockUserSecretRepo()
	h := NewUserHandler(usr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets", nil)
	rec := httptest.NewRecorder()

	h.ListSecrets(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestListSecrets_Empty(t *testing.T) {
	usr := newMockUserSecretRepo()
	h := NewUserHandler(usr)

	user := &model.User{ID: 99, Email: "empty@example.com"}
	ctx := ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ListSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

func TestListSecrets_InvalidPageDefaults(t *testing.T) {
	usr := newMockUserSecretRepo()
	usr.links[1] = []store.SecretSummary{
		{PublicID: "pub1", Label: "first", SecretType: "text", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()},
	}

	h := NewUserHandler(usr)
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)

	// Invalid page value defaults to 1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets?page=abc&per_page=xyz", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ListSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["page"].(float64) != 1 {
		t.Errorf("page = %v, want 1", resp["page"])
	}
	if resp["per_page"].(float64) != 20 {
		t.Errorf("per_page = %v, want 20 (default)", resp["per_page"])
	}
}

func TestListSecrets_NegativePageDefaults(t *testing.T) {
	usr := newMockUserSecretRepo()
	h := NewUserHandler(usr)
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets?page=-1&per_page=0", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ListSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["page"].(float64) != 1 {
		t.Errorf("page = %v, want 1 (default for negative)", resp["page"])
	}
	if resp["per_page"].(float64) != 20 {
		t.Errorf("per_page = %v, want 20 (default for zero)", resp["per_page"])
	}
}

func TestListSecrets_PerPageCappedAt100(t *testing.T) {
	usr := newMockUserSecretRepo()
	h := NewUserHandler(usr)
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets?per_page=200", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ListSecrets(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	// per_page > 100 is rejected, defaults to 20
	if resp["per_page"].(float64) != 20 {
		t.Errorf("per_page = %v, want 20 (default, since 200 > 100)", resp["per_page"])
	}
}

func TestListSecrets_RepoError(t *testing.T) {
	usr := &errorUserSecretRepo{}
	h := NewUserHandler(usr)
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ListSecrets(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestListSecrets_Pagination(t *testing.T) {
	usr := newMockUserSecretRepo()
	for i := range 25 {
		usr.links[1] = append(usr.links[1], store.SecretSummary{
			PublicID: "pub-" + string(rune('A'+i)), Label: "secret", SecretType: "text",
			ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
		})
	}

	h := NewUserHandler(usr)
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)

	// Page 1
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets?page=1&per_page=10", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ListSecrets(rec, req)

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	secrets := resp["secrets"].([]any)
	if len(secrets) != 10 {
		t.Errorf("page 1: got %d secrets, want 10", len(secrets))
	}
	if resp["total"].(float64) != 25 {
		t.Errorf("total = %v, want 25", resp["total"])
	}

	// Page 3
	req = httptest.NewRequest(http.MethodGet, "/api/v1/user/secrets?page=3&per_page=10", nil).WithContext(ctx)
	rec = httptest.NewRecorder()
	h.ListSecrets(rec, req)

	json.NewDecoder(rec.Body).Decode(&resp)
	secrets = resp["secrets"].([]any)
	if len(secrets) != 5 {
		t.Errorf("page 3: got %d secrets, want 5", len(secrets))
	}
}
