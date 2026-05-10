package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

func testMetrics() *metrics.SecretMetrics {
	return metrics.NewSecretMetrics(prometheus.NewRegistry())
}

func newEchoContext(req *http.Request, rec *httptest.ResponseRecorder) echo.Context {
	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler
	return e.NewContext(req, rec)
}

func callHandler(c echo.Context, handler echo.HandlerFunc) {
	if err := handler(c); err != nil {
		c.Echo().HTTPErrorHandler(err, c)
	}
}

// mockSecretRepo implements domain.SecretRepo for testing
type mockSecretRepo struct {
	mu        sync.Mutex
	secrets   map[string]*domain.Secret
	createErr error
}

func newMockRepo() *mockSecretRepo {
	return &mockSecretRepo{secrets: make(map[string]*domain.Secret)}
}

func (m *mockSecretRepo) Create(_ context.Context, s *domain.Secret) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.secrets[s.PublicID]; exists {
		return domain.ErrDuplicate
	}
	secret := *s
	m.secrets[s.PublicID] = &secret
	return nil
}

func (m *mockSecretRepo) GetByPublicID(_ context.Context, publicID string) (*domain.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.secrets[publicID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if s.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrNotFound
	}
	secret := *s
	return &secret, nil
}

func (m *mockSecretRepo) ClaimBurnAfterRead(_ context.Context, publicID, blobToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.secrets[publicID]
	if !ok {
		return domain.ErrNotFound
	}
	if s.ExpiresAt.Before(time.Now()) ||
		s.BlobToken != blobToken ||
		!s.BurnAfterRead ||
		s.RetrievedAt != nil {
		return domain.ErrNotFound
	}

	now := time.Now()
	s.RetrievedAt = &now
	return nil
}

func (m *mockSecretRepo) Delete(_ context.Context, publicID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.secrets[publicID]; !ok {
		return domain.ErrNotFound
	}
	delete(m.secrets, publicID)
	return nil
}

func (m *mockSecretRepo) DeleteExpired(_ context.Context, beforeDelete func(string) error) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for id, s := range m.secrets {
		expired := s.ExpiresAt.Before(time.Now())
		burnedAndRetrieved := s.BurnAfterRead && s.RetrievedAt != nil
		if expired || burnedAndRetrieved {
			if err := beforeDelete(s.PublicID); err != nil {
				continue
			}
			delete(m.secrets, id)
			count++
		}
	}
	return count, nil
}

// mockFileStore implements domain.FileStore for testing
type mockFileStore struct {
	objects   map[string][]byte
	putErr    error
	getErr    error
	deleteErr error
}

func newMockFileStore() *mockFileStore {
	return &mockFileStore{objects: make(map[string][]byte)}
}

func (m *mockFileStore) Put(_ context.Context, key string, reader io.Reader, _ int64) error {
	if m.putErr != nil {
		return m.putErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}

func (m *mockFileStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("object %q not found", key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockFileStore) Delete(_ context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.objects, key)
	return nil
}

// --- Helpers ---

func createMultipartRequest(t *testing.T, fields map[string]string, fileContent []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for k, v := range fields {
		writer.WriteField(k, v)
	}

	if fileContent != nil {
		part, _ := writer.CreateFormFile("file", "blob.bin")
		part.Write(fileContent)
	}

	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func validCreateMetadata() map[string]string {
	return map[string]string{
		"public_id":       "test-public-id",
		"metadata_token":  "dGVzdG1ldGFkYXRhdG9rZW4",
		"blob_token":      "dGVzdGJsb2J0b2tlbg",
		"deletion_token":  "dGVzdGRlbGV0aW9udG9rZW4",
		"encrypted_meta":  "v1$bm9uY2U$Y2lwaGVydGV4dA",
		"expiration":      "7d",
		"burn_after_read": "false",
	}
}

func seedSecret(repo *mockSecretRepo, fs *mockFileStore, publicID, token, deletionToken string, burnAfterRead bool) {
	seedSecretWithTokens(repo, fs, publicID, token, token, deletionToken, burnAfterRead)
}

func seedSecretWithTokens(repo *mockSecretRepo, fs *mockFileStore, publicID, metadataToken, blobToken, deletionToken string, burnAfterRead bool) {
	blobData := []byte("v1$datanonce$encryptedcontent")
	secret := &domain.Secret{
		PublicID:      publicID,
		MetadataToken: metadataToken,
		BlobToken:     blobToken,
		DeletionToken: deletionToken,
		EncryptedMeta: "v1$bm9uY2U$Y2lwaGVydGV4dA",
		BlobSize:      int64(len(blobData)),
		BurnAfterRead: burnAfterRead,
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	fs.objects[storageKey(publicID)] = blobData
	repo.secrets[publicID] = secret
}

// --- Create Tests ---

func TestCreateSecret_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), []byte("encrypted-blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if _, ok := resp["expires_at"]; !ok {
		t.Error("response missing expires_at")
	}

	// Verify S3 object was stored
	if _, ok := fs.objects["secrets/test-public-id"]; !ok {
		t.Error("blob not stored in S3")
	}

	// Verify DB record
	if _, ok := repo.secrets["test-public-id"]; !ok {
		t.Error("secret not created in DB")
	}
}

func TestCreateSecret_MissingFields(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	// Only public_id provided — other required fields missing
	meta := map[string]string{"public_id": "only-one-field"}
	req := createMultipartRequest(t, meta, []byte("data"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_MissingFile(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), nil)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_InvalidExpiration(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	meta := validCreateMetadata()
	meta["expiration"] = "99d"

	req := createMultipartRequest(t, meta, []byte("data"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateSecret_DuplicatePublicID(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	// First create
	req := createMultipartRequest(t, validCreateMetadata(), []byte("blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	callHandler(c, h.CreateSecret)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Second create with same public_id
	req = createMultipartRequest(t, validCreateMetadata(), []byte("blob"))
	rec = httptest.NewRecorder()
	c = newEchoContext(req, rec)
	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestCreateSecret_S3PutError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.putErr = fmt.Errorf("S3 connection refused")
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), []byte("blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreateSecret_DBErrorCleansUpS3(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = fmt.Errorf("database timeout")
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), []byte("blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Verify S3 object was cleaned up
	if _, ok := fs.objects["secrets/test-public-id"]; ok {
		t.Error("S3 object should have been cleaned up after DB error")
	}
}

func TestCreateSecret_FileTooLarge(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	maxSize := int64(50) // 50 bytes
	h := NewSecretHandler(repo, fs, maxSize, testMetrics())

	fileData := make([]byte, 2*1024*1024) // 2MB > 50 bytes + 1MB overhead
	req := createMultipartRequest(t, validCreateMetadata(), fileData)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateSecret_FilePartExceedsMaxFileSize(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	maxSize := int64(50)
	h := NewSecretHandler(repo, fs, maxSize, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), bytes.Repeat([]byte("a"), int(maxSize)+1))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, ok := fs.objects["secrets/test-public-id"]; ok {
		t.Error("oversized blob should not be stored")
	}
	if _, ok := repo.secrets["test-public-id"]; ok {
		t.Error("oversized blob should not create DB record")
	}
}

func TestCreateSecret_FilePartAtMaxFileSizeSucceeds(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	maxSize := int64(50)
	h := NewSecretHandler(repo, fs, maxSize, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), bytes.Repeat([]byte("a"), int(maxSize)))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if got := repo.secrets["test-public-id"].BlobSize; got != maxSize {
		t.Errorf("blob size = %d, want %d", got, maxSize)
	}
}

func TestCreateSecret_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = errors.New("database connection lost")
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := createMultipartRequest(t, validCreateMetadata(), []byte("blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// --- Retrieve Tests ---

func TestRetrieveSecret_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "pub1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/pub1", nil)
	req.Header.Set(HeaderBlobToken, "retrieval-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("pub1")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/octet-stream")
	}

	body := rec.Body.String()
	if body != "v1$datanonce$encryptedcontent" {
		t.Errorf("body = %q, want %q", body, "v1$datanonce$encryptedcontent")
	}
}

func TestRetrieveSecret_InvalidToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "pub1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/pub1", nil)
	req.Header.Set(HeaderBlobToken, "wrong-token")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("pub1")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecret_MetadataTokenCannotFetchBlob(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecretWithTokens(repo, fs, "split-token", "metadata-tok", "blob-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/split-token", nil)
	req.Header.Set(HeaderBlobToken, "metadata-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("split-token")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecret_BurnAfterRead_InvalidTokenDoesNotClaim(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "burn-invalid", "retrieval-tok", "deletion-tok", true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn-invalid", nil)
	req.Header.Set(HeaderBlobToken, "wrong-token")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("burn-invalid")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if repo.secrets["burn-invalid"].RetrievedAt != nil {
		t.Error("retrieved_at should not be set for invalid blob token")
	}
}

func TestRetrieveSecret_NotFound(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/nonexistent", nil)
	req.Header.Set(HeaderBlobToken, "some-token")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("nonexistent")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRetrieveSecret_MissingToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/pub1", nil)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("pub1")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRetrieveSecret_BurnAfterRead(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "burn1", "retrieval-tok", "deletion-tok", true)

	// First retrieval succeeds
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn1", nil)
	req.Header.Set(HeaderBlobToken, "retrieval-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("burn1")
	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusOK {
		t.Errorf("first retrieval: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// DB record and S3 object still exist (soft-delete, not hard-delete)
	if _, ok := repo.secrets["burn1"]; !ok {
		t.Error("secret should still exist after burn-after-read (soft delete)")
	}
	if _, ok := fs.objects["secrets/burn1"]; !ok {
		t.Error("S3 object should still exist after burn-after-read (soft delete)")
	}

	// RetrievedAt should be set
	if repo.secrets["burn1"].RetrievedAt == nil {
		t.Error("retrieved_at should be set after burn-after-read retrieval")
	}

	// Second retrieval fails (already burned)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn1", nil)
	req.Header.Set(HeaderBlobToken, "retrieval-tok")
	rec = httptest.NewRecorder()
	c = newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("burn1")
	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusNotFound {
		t.Errorf("second retrieval: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRetrieveSecret_BurnAfterRead_ConcurrentRevealOnlyOneSucceeds(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "burn-concurrent", "retrieval-tok", "deletion-tok", true)

	const requests = 8
	var wg sync.WaitGroup
	statuses := make(chan int, requests)

	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn-concurrent", nil)
			req.Header.Set(HeaderBlobToken, "retrieval-tok")
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("publicID")
			c.SetParamValues("burn-concurrent")

			callHandler(c, h.RetrieveSecret)
			statuses <- rec.Code
		}()
	}

	wg.Wait()
	close(statuses)

	var okCount, notFoundCount int
	for status := range statuses {
		switch status {
		case http.StatusOK:
			okCount++
		case http.StatusNotFound:
			notFoundCount++
		default:
			t.Fatalf("unexpected status %d", status)
		}
	}

	if okCount != 1 {
		t.Errorf("successful retrievals = %d, want 1", okCount)
	}
	if notFoundCount != requests-1 {
		t.Errorf("not found retrievals = %d, want %d", notFoundCount, requests-1)
	}
}

func TestSecretMetadata_BurnAfterRead_AlreadyRetrieved(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "burn-meta", "retrieval-tok", "deletion-tok", true)

	// Mark as already retrieved
	now := time.Now()
	repo.secrets["burn-meta"].RetrievedAt = &now

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/burn-meta/meta", nil)
	req.Header.Set(HeaderMetadataToken, "retrieval-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("burn-meta")

	callHandler(c, h.SecretMetadata)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSecretMetadata_BlobTokenCannotFetchMetadata(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecretWithTokens(repo, fs, "split-meta", "metadata-tok", "blob-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/split-meta/meta", nil)
	req.Header.Set(HeaderMetadataToken, "blob-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("split-meta")

	callHandler(c, h.SecretMetadata)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecret_MissingPublicID(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/", nil)
	req.Header.Set(HeaderBlobToken, "tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRetrieveSecret_S3GetError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.getErr = fmt.Errorf("S3 not reachable")
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "s3-err", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/s3-err", nil)
	req.Header.Set(HeaderBlobToken, "ret-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("s3-err")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRetrieveSecret_BurnAfterRead_S3GetErrorDoesNotClaim(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.getErr = fmt.Errorf("S3 not reachable")
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "burn-s3-err", "ret-tok", "del-tok", true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn-s3-err", nil)
	req.Header.Set(HeaderBlobToken, "ret-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("burn-s3-err")

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if repo.secrets["burn-s3-err"].RetrievedAt != nil {
		t.Error("retrieved_at should not be set when S3 object cannot be opened")
	}
}

// --- Delete Tests ---

func TestDeleteSecret_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "del1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.Header.Set(HeaderMetadataToken, "retrieval-tok")
	req.Header.Set(HeaderDeletionToken, "deletion-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("del1")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify S3 object was deleted
	if _, ok := fs.objects["secrets/del1"]; ok {
		t.Error("S3 object should have been deleted")
	}
}

func TestDeleteSecret_InvalidDeletionToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "del1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.Header.Set(HeaderMetadataToken, "retrieval-tok")
	req.Header.Set(HeaderDeletionToken, "wrong-token")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("del1")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteSecret_MissingDeletionToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "del1", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.Header.Set(HeaderMetadataToken, "retrieval-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("del1")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_NotFound(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/nonexistent", nil)
	req.Header.Set(HeaderMetadataToken, "tok")
	req.Header.Set(HeaderDeletionToken, "tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("nonexistent")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteSecret_MissingMetadataToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del1", nil)
	req.Header.Set(HeaderDeletionToken, "tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("del1")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_MissingPublicID(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/", nil)
	req.Header.Set(HeaderMetadataToken, "tok")
	req.Header.Set(HeaderDeletionToken, "tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_InvalidMetadataToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "del2", "retrieval-tok", "deletion-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del2", nil)
	req.Header.Set(HeaderMetadataToken, "wrong-retrieval")
	req.Header.Set(HeaderDeletionToken, "deletion-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("del2")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteSecret_S3DeleteError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.deleteErr = errors.New("S3 connection failed")
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	seedSecret(repo, fs, "del-s3-err", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del-s3-err", nil)
	req.Header.Set(HeaderMetadataToken, "ret-tok")
	req.Header.Set(HeaderDeletionToken, "del-tok")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues("del-s3-err")

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if _, ok := repo.secrets["del-s3-err"]; !ok {
		t.Error("secret row should remain when S3 delete fails")
	}
	if _, ok := fs.objects["secrets/del-s3-err"]; !ok {
		t.Error("S3 object should remain when delete fails")
	}
}

// --- parseExpiration ---

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
