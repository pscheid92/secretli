package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
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
	sessions  map[string]mockRetrievalSession
	createErr error
}

type mockRetrievalSession struct {
	publicID  string
	expiresAt time.Time
}

func newMockRepo() *mockSecretRepo {
	return &mockSecretRepo{
		secrets:  make(map[string]*domain.Secret),
		sessions: make(map[string]mockRetrievalSession),
	}
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

func (m *mockSecretRepo) ClaimBurnAfterRead(_ context.Context, publicID, blobTokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.secrets[publicID]
	if !ok {
		return domain.ErrNotFound
	}
	if s.ExpiresAt.Before(time.Now()) ||
		s.BlobTokenHash != blobTokenHash ||
		!s.BurnAfterRead ||
		s.RetrievedAt != nil {
		return domain.ErrNotFound
	}

	now := time.Now()
	s.RetrievedAt = &now
	return nil
}

func (m *mockSecretRepo) StartRetrievalSession(_ context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt time.Time) (*domain.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.secrets[publicID]
	if !ok || s.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrNotFound
	}
	if s.BurnAfterRead && s.RetrievedAt != nil {
		return nil, domain.ErrNotFound
	}
	if !tokencrypto.TokensEqual(blobTokenHash, s.BlobTokenHash) {
		return nil, domain.ErrForbidden
	}
	if s.BurnAfterRead {
		now := time.Now()
		s.RetrievedAt = &now
	}
	m.sessions[sessionTokenHash] = mockRetrievalSession{publicID: publicID, expiresAt: expiresAt}
	secret := *s
	return &secret, nil
}

func (m *mockSecretRepo) GetByRetrievalSession(_ context.Context, publicID, sessionTokenHash string) (*domain.Secret, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionTokenHash]
	if !ok || session.publicID != publicID || session.expiresAt.Before(time.Now()) {
		return nil, domain.ErrForbidden
	}
	s, ok := m.secrets[publicID]
	if !ok || s.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrForbidden
	}
	secret := *s
	return &secret, nil
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

func (m *mockSecretRepo) DeleteExpiredRetrievalSessions(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for tokenHash, session := range m.sessions {
		if session.expiresAt.Before(time.Now()) {
			delete(m.sessions, tokenHash)
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

func (m *mockFileStore) GetRange(_ context.Context, key string, start, end int64) (io.ReadCloser, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("object %q not found", key)
	}
	if start < 0 || end < start || end >= int64(len(data)) {
		return nil, fmt.Errorf("range %d-%d out of bounds", start, end)
	}
	return io.NopCloser(bytes.NewReader(data[start : end+1])), nil
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

func startTestRetrievalSession(t *testing.T, h *SecretHandler, publicID, blobToken string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.StartRetrievalSession)

	if rec.Code != http.StatusCreated {
		t.Fatalf("start retrieval session status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var body struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode retrieval session: %v", err)
	}
	return body.SessionToken
}

func testPublicID(label string) string {
	sum := sha256.Sum256([]byte("public_id:" + label))
	return base64.RawURLEncoding.EncodeToString(sum[:16])
}

func testToken(label string) string {
	sum := sha256.Sum256([]byte("token:" + label))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func testEncryptedMetaV1() string {
	nonce := base64.RawURLEncoding.EncodeToString([]byte("123456789012"))
	ciphertext := base64.RawURLEncoding.EncodeToString([]byte("ciphertext"))
	return "v1$" + nonce + "$" + ciphertext
}

func validCreateMetadata() map[string]string {
	return map[string]string{
		"public_id":       testPublicID("create"),
		"metadata_token":  testToken("create metadata"),
		"blob_token":      testToken("create blob"),
		"deletion_token":  testToken("create deletion"),
		"encrypted_meta":  testEncryptedMetaV1(),
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
		PublicID:          publicID,
		MetadataTokenHash: tokencrypto.TokenHash(metadataToken),
		BlobTokenHash:     tokencrypto.TokenHash(blobToken),
		DeletionTokenHash: tokencrypto.TokenHash(deletionToken),
		EncryptedMeta:     testEncryptedMetaV1(),
		BlobSize:          int64(len(blobData)),
		BurnAfterRead:     burnAfterRead,
		ExpiresAt:         time.Now().Add(time.Hour),
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
	if _, ok := fs.objects[storageKey(testPublicID("create"))]; !ok {
		t.Error("blob not stored in S3")
	}

	// Verify DB record
	secret, ok := repo.secrets[testPublicID("create")]
	if !ok {
		t.Fatal("secret not created in DB")
	}
	meta := validCreateMetadata()
	if secret.MetadataTokenHash != tokencrypto.TokenHash(meta["metadata_token"]) {
		t.Error("metadata token should be stored as a hash")
	}
	if secret.BlobTokenHash != tokencrypto.TokenHash(meta["blob_token"]) {
		t.Error("blob token should be stored as a hash")
	}
	if secret.DeletionTokenHash != tokencrypto.TokenHash(meta["deletion_token"]) {
		t.Error("deletion token should be stored as a hash")
	}
}

func TestCreateSecret_MissingFields(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	// Only public_id provided — other required fields missing
	meta := map[string]string{"public_id": testPublicID("missing fields")}
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

func TestCreateSecret_RejectsMalformedProtectedFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
	}{
		{name: "public id", field: "public_id", value: "short"},
		{name: "metadata token", field: "metadata_token", value: "short"},
		{name: "blob token", field: "blob_token", value: "short"},
		{name: "deletion token", field: "deletion_token", value: "short"},
		{name: "encrypted meta", field: "encrypted_meta", value: "v1$bad$bad"},
		{name: "oversized encrypted meta", field: "encrypted_meta", value: string(bytes.Repeat([]byte("A"), domain.EncryptedMetaMaxBytes+1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			fs := newMockFileStore()
			h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

			meta := validCreateMetadata()
			meta[tt.field] = tt.value

			req := createMultipartRequest(t, meta, []byte("data"))
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)

			callHandler(c, h.CreateSecret)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if len(repo.secrets) != 0 {
				t.Error("malformed request should not create DB record")
			}
			if len(fs.objects) != 0 {
				t.Error("malformed request should not store blob")
			}
		})
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
	if _, ok := fs.objects[storageKey(testPublicID("create"))]; ok {
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
	if _, ok := fs.objects[storageKey(testPublicID("create"))]; ok {
		t.Error("oversized blob should not be stored")
	}
	if _, ok := repo.secrets[testPublicID("create")]; ok {
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
	if got := repo.secrets[testPublicID("create")].BlobSize; got != maxSize {
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
	publicID := testPublicID("retrieve success")
	blobToken := testToken("retrieve blob")
	deletionToken := testToken("retrieve deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

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
	publicID := testPublicID("retrieve invalid token")
	blobToken := testToken("retrieve blob")
	deletionToken := testToken("retrieve deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, testToken("wrong blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecret_MetadataTokenCannotFetchBlob(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("retrieve split")
	metadataToken := testToken("split metadata")
	blobToken := testToken("split blob")
	deletionToken := testToken("split deletion")
	seedSecretWithTokens(repo, fs, publicID, metadataToken, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, metadataToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecret_BurnAfterRead_InvalidTokenDoesNotClaim(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("burn invalid token")
	blobToken := testToken("burn invalid blob")
	deletionToken := testToken("burn invalid deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, testToken("wrong blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if repo.secrets[publicID].RetrievedAt != nil {
		t.Error("retrieved_at should not be set for invalid blob token")
	}
}

func TestRetrieveSecret_NotFound(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("retrieve nonexistent")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, testToken("some token"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRetrieveSecret_MissingToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("missing token")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRetrieveSecret_BurnAfterRead(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("burn after read")
	blobToken := testToken("burn after read blob")
	deletionToken := testToken("burn after read deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, true)

	// First retrieval succeeds
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)
	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusOK {
		t.Errorf("first retrieval: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// DB record and S3 object still exist (soft-delete, not hard-delete)
	if _, ok := repo.secrets[publicID]; !ok {
		t.Error("secret should still exist after burn-after-read (soft delete)")
	}
	if _, ok := fs.objects[storageKey(publicID)]; !ok {
		t.Error("S3 object should still exist after burn-after-read (soft delete)")
	}

	// RetrievedAt should be set
	if repo.secrets[publicID].RetrievedAt == nil {
		t.Error("retrieved_at should be set after burn-after-read retrieval")
	}

	// Second retrieval fails (already burned)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec = httptest.NewRecorder()
	c = newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)
	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusNotFound {
		t.Errorf("second retrieval: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStartRetrievalSession_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("session success")
	blobToken := testToken("session blob")
	deletionToken := testToken("session deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.StartRetrievalSession)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var body struct {
		SessionToken  string `json:"session_token"`
		BlobSize      int64  `json:"blob_size"`
		BurnAfterRead bool   `json:"burn_after_read"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !domain.ValidToken(body.SessionToken) {
		t.Fatalf("session token has invalid format: %q", body.SessionToken)
	}
	if body.BlobSize != repo.secrets[publicID].BlobSize {
		t.Errorf("blob_size = %d, want %d", body.BlobSize, repo.secrets[publicID].BlobSize)
	}
	if body.BurnAfterRead {
		t.Error("burn_after_read = true, want false")
	}
}

func TestStartRetrievalSession_InvalidBlobToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("session invalid blob")
	blobToken := testToken("session invalid blob token")
	deletionToken := testToken("session invalid deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", nil)
	req.Header.Set(HeaderBlobToken, testToken("wrong session blob"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.StartRetrievalSession)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if len(repo.sessions) != 0 {
		t.Error("invalid blob token should not create a retrieval session")
	}
}

func TestStartRetrievalSession_BurnAfterReadClaimsOnce(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("session burn")
	blobToken := testToken("session burn blob")
	deletionToken := testToken("session burn deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.StartRetrievalSession)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if repo.secrets[publicID].RetrievedAt == nil {
		t.Fatal("retrieved_at should be set")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec = httptest.NewRecorder()
	c = newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.StartRetrievalSession)

	if rec.Code != http.StatusNotFound {
		t.Errorf("second session status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRetrieveSecretRange_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("range success")
	blobToken := testToken("range blob")
	deletionToken := testToken("range deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	sessionToken := startTestRetrievalSession(t, h, publicID, blobToken)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/blob", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Range", "bytes=3-8")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecretRange)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusPartialContent, rec.Body.String())
	}
	if got, want := rec.Header().Get("Content-Range"), "bytes 3-8/29"; got != want {
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Accept-Ranges"), "bytes"; got != want {
		t.Errorf("Accept-Ranges = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Content-Length"), "6"; got != want {
		t.Errorf("Content-Length = %q, want %q", got, want)
	}
	if got, want := rec.Body.String(), "datano"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestRetrieveSecretRange_InvalidSession(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("range invalid session")
	blobToken := testToken("range invalid blob")
	deletionToken := testToken("range invalid deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/blob", nil)
	req.Header.Set("Authorization", "Bearer "+testToken("wrong session"))
	req.Header.Set("Range", "bytes=0-1")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecretRange)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRetrieveSecretRange_ExpiredSession(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("range expired session")
	blobToken := testToken("range expired blob")
	sessionToken := testToken("range expired session token")
	deletionToken := testToken("range expired deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)
	repo.sessions[tokencrypto.TokenHash(sessionToken)] = mockRetrievalSession{
		publicID:  publicID,
		expiresAt: time.Now().Add(-time.Minute),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/blob", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Range", "bytes=0-1")
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecretRange)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRetrieveSecretRange_AuthorizationValidation(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("range auth validation")

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing", authorization: ""},
		{name: "wrong scheme", authorization: "Token " + testToken("range wrong scheme")},
		{name: "missing token", authorization: "Bearer "},
		{name: "extra segment", authorization: "Bearer " + testToken("range auth extra") + " extra"},
		{name: "malformed token", authorization: "Bearer short"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/blob", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			req.Header.Set("Range", "bytes=0-1")
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("publicID")
			c.SetParamValues(publicID)

			callHandler(c, h.RetrieveSecretRange)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestRetrieveSecretRange_RangeValidation(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("range validation")
	blobToken := testToken("range validation blob")
	deletionToken := testToken("range validation deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)
	sessionToken := startTestRetrievalSession(t, h, publicID, blobToken)

	tests := []struct {
		name             string
		rangeValue       string
		wantStatus       int
		wantContentRange string
	}{
		{name: "missing", rangeValue: "", wantStatus: http.StatusBadRequest},
		{name: "malformed", rangeValue: "bytes=1-", wantStatus: http.StatusBadRequest},
		{name: "multi range", rangeValue: "bytes=1-2,3-4", wantStatus: http.StatusBadRequest},
		{name: "out of bounds", rangeValue: "bytes=0-99", wantStatus: http.StatusRequestedRangeNotSatisfiable, wantContentRange: "bytes */29"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/blob", nil)
			req.Header.Set("Authorization", "Bearer "+sessionToken)
			req.Header.Set("Range", tt.rangeValue)
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("publicID")
			c.SetParamValues(publicID)

			callHandler(c, h.RetrieveSecretRange)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Range"); got != tt.wantContentRange {
				t.Errorf("Content-Range = %q, want %q", got, tt.wantContentRange)
			}
		})
	}
}

func TestRetrieveSecret_BurnAfterRead_ConcurrentRevealOnlyOneSucceeds(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("burn concurrent")
	blobToken := testToken("burn concurrent blob")
	deletionToken := testToken("burn concurrent deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, true)

	const requests = 8
	var wg sync.WaitGroup
	statuses := make(chan int, requests)

	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
			req.Header.Set(HeaderBlobToken, blobToken)
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("publicID")
			c.SetParamValues(publicID)

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

func TestSecretMetadata_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("metadata success")
	metadataToken := testToken("metadata success token")
	deletionToken := testToken("metadata success deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/meta", nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.SecretMetadata)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp domain.SecretMetadataResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.EncryptedMeta != testEncryptedMetaV1() {
		t.Errorf("encrypted_meta = %q, want fixture", resp.EncryptedMeta)
	}
	if resp.BlobSize == 0 {
		t.Error("blob_size should be populated")
	}
}

func TestSecretMetadata_BurnAfterRead_AlreadyRetrieved(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("burn metadata")
	metadataToken := testToken("burn metadata token")
	deletionToken := testToken("burn metadata deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, true)

	// Mark as already retrieved
	now := time.Now()
	repo.secrets[publicID].RetrievedAt = &now

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/meta", nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.SecretMetadata)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSecretMetadata_BlobTokenCannotFetchMetadata(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("metadata split")
	metadataToken := testToken("metadata split metadata")
	blobToken := testToken("metadata split blob")
	deletionToken := testToken("metadata split deletion")
	seedSecretWithTokens(repo, fs, publicID, metadataToken, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/meta", nil)
	req.Header.Set(HeaderMetadataToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

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
	req.Header.Set(HeaderBlobToken, testToken("missing public id"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSecretHandlers_MalformedPublicID(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		pathSuffix string
		setup      func(*http.Request)
		handler    func(*SecretHandler) echo.HandlerFunc
	}{
		{
			name:       "retrieve",
			method:     http.MethodPost,
			pathSuffix: "",
			setup: func(req *http.Request) {
				req.Header.Set(HeaderBlobToken, testToken("malformed public retrieve"))
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.RetrieveSecret },
		},
		{
			name:       "metadata",
			method:     http.MethodGet,
			pathSuffix: "/meta",
			setup: func(req *http.Request) {
				req.Header.Set(HeaderMetadataToken, testToken("malformed public metadata"))
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.SecretMetadata },
		},
		{
			name:       "start retrieval session",
			method:     http.MethodPost,
			pathSuffix: "/retrieval-session",
			setup: func(req *http.Request) {
				req.Header.Set(HeaderBlobToken, testToken("malformed public session"))
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.StartRetrievalSession },
		},
		{
			name:       "retrieve range",
			method:     http.MethodGet,
			pathSuffix: "/blob",
			setup: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer "+testToken("malformed public range"))
				req.Header.Set("Range", "bytes=0-1")
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.RetrieveSecretRange },
		},
		{
			name:       "delete",
			method:     http.MethodDelete,
			pathSuffix: "",
			setup: func(req *http.Request) {
				req.Header.Set(HeaderMetadataToken, testToken("malformed public delete metadata"))
				req.Header.Set(HeaderDeletionToken, testToken("malformed public delete deletion"))
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.DeleteSecret },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			fs := newMockFileStore()
			h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
			publicID := "short"

			req := httptest.NewRequest(tt.method, "/api/v1/secrets/"+publicID+tt.pathSuffix, nil)
			tt.setup(req)
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("publicID")
			c.SetParamValues(publicID)

			callHandler(c, tt.handler(h))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestSecretHandlers_MalformedTokenHeaders(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    func(string) string
		setup   func(*mockSecretRepo, *mockFileStore, string, *http.Request)
		handler func(*SecretHandler) echo.HandlerFunc
	}{
		{
			name:   "retrieve blob token",
			method: http.MethodPost,
			path:   func(publicID string) string { return "/api/v1/secrets/" + publicID },
			setup: func(_ *mockSecretRepo, _ *mockFileStore, _ string, req *http.Request) {
				req.Header.Set(HeaderBlobToken, "short")
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.RetrieveSecret },
		},
		{
			name:   "start retrieval session blob token",
			method: http.MethodPost,
			path:   func(publicID string) string { return "/api/v1/secrets/" + publicID + "/retrieval-session" },
			setup: func(_ *mockSecretRepo, _ *mockFileStore, _ string, req *http.Request) {
				req.Header.Set(HeaderBlobToken, "short")
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.StartRetrievalSession },
		},
		{
			name:   "retrieve range authorization token",
			method: http.MethodGet,
			path:   func(publicID string) string { return "/api/v1/secrets/" + publicID + "/blob" },
			setup: func(_ *mockSecretRepo, _ *mockFileStore, _ string, req *http.Request) {
				req.Header.Set("Authorization", "Bearer short")
				req.Header.Set("Range", "bytes=0-1")
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.RetrieveSecretRange },
		},
		{
			name:   "metadata token",
			method: http.MethodGet,
			path:   func(publicID string) string { return "/api/v1/secrets/" + publicID + "/meta" },
			setup: func(_ *mockSecretRepo, _ *mockFileStore, _ string, req *http.Request) {
				req.Header.Set(HeaderMetadataToken, "short")
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.SecretMetadata },
		},
		{
			name:   "delete metadata token",
			method: http.MethodDelete,
			path:   func(publicID string) string { return "/api/v1/secrets/" + publicID },
			setup: func(_ *mockSecretRepo, _ *mockFileStore, _ string, req *http.Request) {
				req.Header.Set(HeaderMetadataToken, "short")
				req.Header.Set(HeaderDeletionToken, testToken("delete malformed metadata deletion"))
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.DeleteSecret },
		},
		{
			name:   "delete deletion token",
			method: http.MethodDelete,
			path:   func(publicID string) string { return "/api/v1/secrets/" + publicID },
			setup: func(repo *mockSecretRepo, fs *mockFileStore, publicID string, req *http.Request) {
				metadataToken := testToken("delete malformed deletion metadata")
				seedSecret(repo, fs, publicID, metadataToken, testToken("delete malformed deletion"), false)
				req.Header.Set(HeaderMetadataToken, metadataToken)
				req.Header.Set(HeaderDeletionToken, "short")
			},
			handler: func(h *SecretHandler) echo.HandlerFunc { return h.DeleteSecret },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			fs := newMockFileStore()
			h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
			publicID := testPublicID("malformed token " + tt.name)

			req := httptest.NewRequest(tt.method, tt.path(publicID), nil)
			tt.setup(repo, fs, publicID, req)
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("publicID")
			c.SetParamValues(publicID)

			callHandler(c, tt.handler(h))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestRetrieveSecret_S3GetError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.getErr = fmt.Errorf("S3 not reachable")
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("s3 get error")
	blobToken := testToken("s3 get blob")
	deletionToken := testToken("s3 get deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

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
	publicID := testPublicID("burn s3 get error")
	blobToken := testToken("burn s3 blob")
	deletionToken := testToken("burn s3 deletion")
	seedSecret(repo, fs, publicID, blobToken, deletionToken, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.RetrieveSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if repo.secrets[publicID].RetrievedAt != nil {
		t.Error("retrieved_at should not be set when S3 object cannot be opened")
	}
}

// --- Delete Tests ---

func TestDeleteSecret_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("delete success")
	metadataToken := testToken("delete metadata")
	deletionToken := testToken("delete deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	req.Header.Set(HeaderDeletionToken, deletionToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify S3 object was deleted
	if _, ok := fs.objects[storageKey(publicID)]; ok {
		t.Error("S3 object should have been deleted")
	}
}

func TestDeleteSecret_InvalidDeletionToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("delete invalid deletion")
	metadataToken := testToken("delete metadata")
	deletionToken := testToken("delete deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	req.Header.Set(HeaderDeletionToken, testToken("wrong deletion"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteSecret_MissingDeletionToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("delete missing deletion")
	metadataToken := testToken("delete metadata")
	deletionToken := testToken("delete deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteSecret_NotFound(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())
	publicID := testPublicID("delete nonexistent")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, testToken("delete metadata"))
	req.Header.Set(HeaderDeletionToken, testToken("delete deletion"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteSecret_MissingMetadataToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, 100*1024*1024, testMetrics())

	publicID := testPublicID("delete missing metadata")
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderDeletionToken, testToken("delete deletion"))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

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
	req.Header.Set(HeaderMetadataToken, testToken("missing delete public id metadata"))
	req.Header.Set(HeaderDeletionToken, testToken("missing delete public id deletion"))
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
	publicID := testPublicID("delete invalid metadata")
	metadataToken := testToken("delete metadata")
	deletionToken := testToken("delete deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, testToken("wrong metadata"))
	req.Header.Set(HeaderDeletionToken, deletionToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

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
	publicID := testPublicID("delete s3 error")
	metadataToken := testToken("delete s3 metadata")
	deletionToken := testToken("delete s3 deletion")
	seedSecret(repo, fs, publicID, metadataToken, deletionToken, false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	req.Header.Set(HeaderDeletionToken, deletionToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)

	callHandler(c, h.DeleteSecret)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if _, ok := repo.secrets[publicID]; !ok {
		t.Error("secret row should remain when S3 delete fails")
	}
	if _, ok := fs.objects[storageKey(publicID)]; !ok {
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
