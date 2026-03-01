package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/crypto"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

// mockSecretRepo implements store.SecretRepo for testing
type mockSecretRepo struct {
	secrets map[string]*model.Secret
	createErr error
}

func newMockRepo() *mockSecretRepo {
	return &mockSecretRepo{secrets: make(map[string]*model.Secret)}
}

func (m *mockSecretRepo) Create(_ context.Context, s *model.Secret) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.secrets[s.PublicID]; exists {
		return store.ErrDuplicate
	}
	m.secrets[s.PublicID] = s
	return nil
}

func (m *mockSecretRepo) GetByPublicID(_ context.Context, publicID string) (*model.Secret, error) {
	s, ok := m.secrets[publicID]
	if !ok {
		return nil, store.ErrNotFound
	}
	if s.ExpiresAt.Before(time.Now()) {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func (m *mockSecretRepo) GetAndDeleteByPublicID(_ context.Context, publicID string) (*model.Secret, error) {
	s, ok := m.secrets[publicID]
	if !ok {
		return nil, store.ErrNotFound
	}
	delete(m.secrets, publicID)
	return s, nil
}

func (m *mockSecretRepo) SetRetrievedAt(_ context.Context, publicID string) error {
	if s, ok := m.secrets[publicID]; ok {
		now := time.Now()
		s.RetrievedAt = &now
	}
	return nil
}

func (m *mockSecretRepo) Delete(_ context.Context, publicID string) error {
	if _, ok := m.secrets[publicID]; !ok {
		return store.ErrNotFound
	}
	delete(m.secrets, publicID)
	return nil
}

func (m *mockSecretRepo) DeleteExpired(_ context.Context) (int64, []string, error) {
	var count int64
	var keys []string
	for id, s := range m.secrets {
		if s.ExpiresAt.Before(time.Now()) {
			delete(m.secrets, id)
			count++
			if s.StorageKey != nil {
				keys = append(keys, *s.StorageKey)
			}
		}
	}
	return count, keys, nil
}

// mockUserSecretRepoForSecret tracks LinkSecret calls
type mockUserSecretRepoForSecret struct {
	linked []struct {
		userID   int64
		secretID int64
		label    string
	}
}

func (m *mockUserSecretRepoForSecret) LinkSecret(_ context.Context, userID, secretID int64, label string) error {
	m.linked = append(m.linked, struct {
		userID   int64
		secretID int64
		label    string
	}{userID, secretID, label})
	return nil
}

func (m *mockUserSecretRepoForSecret) ListByUser(_ context.Context, _ int64, _, _ int) ([]store.SecretSummary, int64, error) {
	return nil, 0, nil
}

// Helper to create a valid request body
func validCreateBody() map[string]any {
	return map[string]any{
		"public_id":          "test-public-id",
		"retrieval_token":    "dGVzdHJldHJpZXZhbHRva2Vu",
		"deletion_token":     "dGVzdGRlbGV0aW9udG9rZW4",
		"nonce":              "dGVzdG5vbmNl",
		"encrypted_data":     "ZW5jcnlwdGVkZGF0YQ",
		"expiration":         "7d",
		"burn_after_read":    false,
		"password_protected": false,
	}
}

func TestCreateSecret_Success(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	body, _ := json.Marshal(validCreateBody())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if _, ok := resp["expires_at"]; !ok {
		t.Error("response missing expires_at")
	}
}

func TestCreateSecret_MissingFields(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	body, _ := json.Marshal(map[string]string{"public_id": "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_InvalidExpiration(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	b := validCreateBody()
	b["expiration"] = "99d"
	body, _ := json.Marshal(b)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_DuplicatePublicID(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	body, _ := json.Marshal(validCreateBody())

	// First create
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.CreateSecret(rec, req)

	// Second create with same public_id
	body, _ = json.Marshal(validCreateBody())
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	h.CreateSecret(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

// Helper to seed a secret into the mock repo
func seedSecret(repo *mockSecretRepo, publicID, retrievalToken, deletionToken string, burnAfterRead bool) {
	encData := "ZW5jcnlwdGVkZGF0YQ"
	repo.secrets[publicID] = &model.Secret{
		PublicID:           publicID,
		RetrievalTokenHash: crypto.HashToken(retrievalToken),
		DeletionTokenHash:  crypto.HashToken(deletionToken),
		EncryptedData:      &encData,
		Nonce:              "dGVzdG5vbmNl",
		SecretType:         "text",
		BurnAfterRead:      burnAfterRead,
		ExpiresAt:          time.Now().Add(time.Hour),
	}
}

func TestRetrieveSecret_Success(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)
	seedSecret(repo, "pub1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/pub1", nil)
	req.SetPathValue("publicID", "pub1")
	req.Header.Set("X-Retrieval-Token", "retrieval-tok")
	rec := httptest.NewRecorder()

	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["nonce"] == "" {
		t.Error("response missing nonce")
	}
	if resp["encrypted_data"] == "" {
		t.Error("response missing encrypted_data")
	}
}

func TestRetrieveSecret_InvalidToken(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)
	seedSecret(repo, "pub1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/pub1", nil)
	req.SetPathValue("publicID", "pub1")
	req.Header.Set("X-Retrieval-Token", "wrong-token")
	rec := httptest.NewRecorder()

	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecret_NotFound(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/nonexistent", nil)
	req.SetPathValue("publicID", "nonexistent")
	req.Header.Set("X-Retrieval-Token", "some-token")
	rec := httptest.NewRecorder()

	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRetrieveSecret_MissingToken(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/pub1", nil)
	req.SetPathValue("publicID", "pub1")
	rec := httptest.NewRecorder()

	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRetrieveSecret_BurnAfterRead(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)
	seedSecret(repo, "burn1", "retrieval-tok", "deletion-tok", true)

	// First retrieval succeeds
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn1", nil)
	req.SetPathValue("publicID", "burn1")
	req.Header.Set("X-Retrieval-Token", "retrieval-tok")
	rec := httptest.NewRecorder()
	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("first retrieval: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Second retrieval fails
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn1", nil)
	req.SetPathValue("publicID", "burn1")
	req.Header.Set("X-Retrieval-Token", "retrieval-tok")
	rec = httptest.NewRecorder()
	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("second retrieval: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteSecret_Success(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)
	seedSecret(repo, "del1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.SetPathValue("publicID", "del1")
	req.Header.Set("X-Retrieval-Token", "retrieval-tok")
	req.Header.Set("X-Deletion-Token", "deletion-tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestDeleteSecret_InvalidDeletionToken(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)
	seedSecret(repo, "del1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.SetPathValue("publicID", "del1")
	req.Header.Set("X-Retrieval-Token", "retrieval-tok")
	req.Header.Set("X-Deletion-Token", "wrong-token")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteSecret_MissingDeletionToken(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.SetPathValue("publicID", "del1")
	req.Header.Set("X-Retrieval-Token", "retrieval-tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_NotFound(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/nonexistent", nil)
	req.SetPathValue("publicID", "nonexistent")
	req.Header.Set("X-Retrieval-Token", "tok")
	req.Header.Set("X-Deletion-Token", "tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestParseExpiration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"5m", 5 * time.Minute, false},
		{"10m", 10 * time.Minute, false},
		{"15m", 15 * time.Minute, false},
		{"1h", time.Hour, false},
		{"4h", 4 * time.Hour, false},
		{"12h", 12 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"3d", 72 * time.Hour, false},
		{"7d", 168 * time.Hour, false},
		{"99d", 0, true},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		d, err := parseExpiration(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("parseExpiration(%q) expected error", tt.input)
		}
		if !tt.wantErr && err != nil {
			t.Errorf("parseExpiration(%q) unexpected error: %v", tt.input, err)
		}
		if d != tt.expected {
			t.Errorf("parseExpiration(%q) = %v, want %v", tt.input, d, tt.expected)
		}
	}
}

func TestCreateSecret_LinksToAuthenticatedUser(t *testing.T) {
	repo := newMockRepo()
	usr := &mockUserSecretRepoForSecret{}
	h := NewSecretHandler(repo, nil, usr)

	body, _ := json.Marshal(validCreateBody())
	user := &model.User{ID: 42, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	if len(usr.linked) != 1 {
		t.Fatalf("expected 1 link, got %d", len(usr.linked))
	}
	if usr.linked[0].userID != 42 {
		t.Errorf("linked userID = %d, want 42", usr.linked[0].userID)
	}
}

func TestCreateSecret_InvalidJSON(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader([]byte("{invalid json")))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_EncryptedDataTooLarge(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	b := validCreateBody()
	// Create encrypted_data that exceeds 1MB
	largeData := make([]byte, (1<<20)+1)
	for i := range largeData {
		largeData[i] = 'A'
	}
	b["encrypted_data"] = string(largeData)
	body, _ := json.Marshal(b)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = errors.New("database connection lost")
	h := NewSecretHandler(repo, nil, nil)

	body, _ := json.Marshal(validCreateBody())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRetrieveSecret_MissingPublicID(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/", nil)
	// No path value set
	req.Header.Set("X-Retrieval-Token", "tok")
	rec := httptest.NewRecorder()

	h.RetrieveSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_MissingRetrievalToken(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.SetPathValue("publicID", "del1")
	req.Header.Set("X-Deletion-Token", "tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_MissingPublicID(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/", nil)
	req.Header.Set("X-Retrieval-Token", "tok")
	req.Header.Set("X-Deletion-Token", "tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_InvalidRetrievalToken(t *testing.T) {
	repo := newMockRepo()
	h := NewSecretHandler(repo, nil, nil)
	seedSecret(repo, "del2", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del2", nil)
	req.SetPathValue("publicID", "del2")
	req.Header.Set("X-Retrieval-Token", "wrong-retrieval")
	req.Header.Set("X-Deletion-Token", "deletion-tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteSecret_S3DeleteError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.deleteErr = errors.New("S3 connection failed")
	h := NewSecretHandler(repo, fs, nil)
	seedFileSecret(repo, fs, "del-s3-err", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del-s3-err", nil)
	req.SetPathValue("publicID", "del-s3-err")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	req.Header.Set("X-Deletion-Token", "del-tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	// Should still succeed - S3 error is logged but doesn't block deletion
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestCreateSecret_AnonymousDoesNotLink(t *testing.T) {
	repo := newMockRepo()
	usr := &mockUserSecretRepoForSecret{}
	h := NewSecretHandler(repo, nil, usr)

	body, _ := json.Marshal(validCreateBody())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.CreateSecret(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if len(usr.linked) != 0 {
		t.Errorf("expected 0 links for anonymous, got %d", len(usr.linked))
	}
}
