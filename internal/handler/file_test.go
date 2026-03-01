package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/crypto"
	"github.com/pscheid92/secretli/internal/model"
)

// mockFileStore implements storage.FileStore for testing
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

func createMultipartRequest(t *testing.T, metadata map[string]any, fileContent []byte) (*http.Request, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if metadata != nil {
		metaJSON, _ := json.Marshal(metadata)
		writer.WriteField("metadata", string(metaJSON))
	}

	if fileContent != nil {
		part, _ := writer.CreateFormFile("file", "test.bin")
		part.Write(fileContent)
	}

	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, writer.FormDataContentType()
}

func validFileMetadata() map[string]any {
	return map[string]any{
		"public_id":          "file-pub-id",
		"retrieval_token":    "file-retrieval-tok",
		"deletion_token":     "file-deletion-tok",
		"nonce":              "ZmlsZW5vbmNl",
		"expiration":         "7d",
		"burn_after_read":    false,
		"password_protected": false,
		"encrypted_filename": "abc123:def456",
	}
}

// --- Upload Tests ---

func TestUploadFile_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	fileData := []byte("encrypted-file-content")
	req, _ := createMultipartRequest(t, validFileMetadata(), fileData)
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if _, ok := resp["expires_at"]; !ok {
		t.Error("response missing expires_at")
	}

	// Verify S3 object was stored
	if _, ok := fs.objects["secrets/file-pub-id"]; !ok {
		t.Error("file not stored in S3")
	}

	// Verify DB record
	if _, ok := repo.secrets["file-pub-id"]; !ok {
		t.Error("secret not created in DB")
	}
	if repo.secrets["file-pub-id"].SecretType != "file" {
		t.Errorf("secret_type = %q, want %q", repo.secrets["file-pub-id"].SecretType, "file")
	}
}

func TestUploadFile_MissingMetadata(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	// Create request with file but no metadata
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "test.bin")
	part.Write([]byte("data"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadFile_MissingFile(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	req, _ := createMultipartRequest(t, validFileMetadata(), nil)
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadFile_FileTooLarge(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	maxSize := int64(50) // 50 bytes for testing
	h := NewFileHandler(repo, fs, maxSize, nil)

	// Create a file large enough to exceed MaxBytesReader limit (maxSize + 1MB for metadata)
	// We need the total multipart body to exceed the limit
	fileData := make([]byte, 2*1024*1024) // 2MB > 50 bytes + 1MB overhead
	req, _ := createMultipartRequest(t, validFileMetadata(), fileData)
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUploadFile_DuplicatePublicID(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	fileData := []byte("encrypted-file-content")

	// First upload
	req, _ := createMultipartRequest(t, validFileMetadata(), fileData)
	rec := httptest.NewRecorder()
	h.UploadFile(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first upload: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Second upload with same public_id
	req, _ = createMultipartRequest(t, validFileMetadata(), fileData)
	rec = httptest.NewRecorder()
	h.UploadFile(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

// --- Download Tests ---

func seedFileSecret(repo *mockSecretRepo, fs *mockFileStore, publicID, retrievalToken, deletionToken string, burnAfterRead bool) {
	storageKey := "secrets/" + publicID
	encFilename := "nonce123:cipher456"
	var encSize int64 = 22
	fs.objects[storageKey] = []byte("encrypted-file-content")
	repo.secrets[publicID] = &model.Secret{
		PublicID:           publicID,
		RetrievalTokenHash: crypto.HashToken(retrievalToken),
		DeletionTokenHash:  crypto.HashToken(deletionToken),
		Nonce:              "ZmlsZW5vbmNl",
		SecretType:         "file",
		StorageKey:         &storageKey,
		EncryptedFilename:  &encFilename,
		EncryptedSize:      &encSize,
		BurnAfterRead:      burnAfterRead,
		ExpiresAt:          time.Now().Add(time.Hour),
	}
}

func TestDownloadFile_Success(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)
	seedFileSecret(repo, fs, "file1", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/file1/file", nil)
	req = withChiURLParam(req, "publicID", "file1")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/octet-stream")
	}

	if fn := rec.Header().Get("X-Encrypted-Filename"); fn != "nonce123:cipher456" {
		t.Errorf("X-Encrypted-Filename = %q, want %q", fn, "nonce123:cipher456")
	}

	body := rec.Body.String()
	if body != "encrypted-file-content" {
		t.Errorf("body = %q, want %q", body, "encrypted-file-content")
	}
}

func TestDownloadFile_InvalidToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)
	seedFileSecret(repo, fs, "file1", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/file1/file", nil)
	req = withChiURLParam(req, "publicID", "file1")
	req.Header.Set("X-Retrieval-Token", "wrong-token")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDownloadFile_NotFound(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/nonexistent/file", nil)
	req = withChiURLParam(req, "publicID", "nonexistent")
	req.Header.Set("X-Retrieval-Token", "some-tok")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDownloadFile_WrongSecretType(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)
	seedSecret(repo, "text1", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/text1/file", nil)
	req = withChiURLParam(req, "publicID", "text1")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "not a file type") {
		t.Errorf("body should mention wrong type: %s", rec.Body.String())
	}
}

func TestDownloadFile_BurnAfterRead(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)
	seedFileSecret(repo, fs, "burn-file", "ret-tok", "del-tok", true)

	// First download succeeds
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn-file/file", nil)
	req = withChiURLParam(req, "publicID", "burn-file")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	rec := httptest.NewRecorder()
	h.DownloadFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("first download: status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify DB record deleted
	if _, ok := repo.secrets["burn-file"]; ok {
		t.Error("secret should have been deleted after burn-after-read")
	}

	// Verify S3 object deleted
	if _, ok := fs.objects["secrets/burn-file"]; ok {
		t.Error("S3 object should have been deleted after burn-after-read")
	}

	// Second download fails
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets/burn-file/file", nil)
	req = withChiURLParam(req, "publicID", "burn-file")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	rec = httptest.NewRecorder()
	h.DownloadFile(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("second download: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// --- DeleteSecret with S3 cleanup ---

func TestDeleteSecret_FileSecretCleansS3(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, nil)
	seedFileSecret(repo, fs, "del-file", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/del-file", nil)
	req = withChiURLParam(req, "publicID", "del-file")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	req.Header.Set("X-Deletion-Token", "del-tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify S3 object was deleted
	if _, ok := fs.objects["secrets/del-file"]; ok {
		t.Error("S3 object should have been deleted")
	}
}

func TestUploadFile_S3PutError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.putErr = fmt.Errorf("S3 connection refused")
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	fileData := []byte("encrypted-file-content")
	req, _ := createMultipartRequest(t, validFileMetadata(), fileData)
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestUploadFile_InvalidMetadataJSON(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("metadata", "{invalid json!!!")
	part, _ := writer.CreateFormFile("file", "test.bin")
	part.Write([]byte("data"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadFile_MissingRequiredMetadataFields(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	meta := map[string]any{"public_id": "only-one-field"}
	req, _ := createMultipartRequest(t, meta, []byte("data"))
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadFile_InvalidExpiration(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	meta := validFileMetadata()
	meta["expiration"] = "99d"
	req, _ := createMultipartRequest(t, meta, []byte("data"))
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadFile_DBErrorCleansUpS3(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = fmt.Errorf("database timeout")
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	fileData := []byte("encrypted-file-content")
	req, _ := createMultipartRequest(t, validFileMetadata(), fileData)
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Verify S3 object was cleaned up
	if _, ok := fs.objects["secrets/file-pub-id"]; ok {
		t.Error("S3 object should have been cleaned up after DB error")
	}
}

func TestDownloadFile_MissingToken(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/file1/file", nil)
	req = withChiURLParam(req, "publicID", "file1")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDownloadFile_MissingPublicID(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets//file", nil)
	req.Header.Set("X-Retrieval-Token", "tok")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDownloadFile_MissingStorageKey(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)

	// Create a file secret with nil StorageKey
	repo.secrets["no-key"] = &model.Secret{
		PublicID:           "no-key",
		RetrievalTokenHash: crypto.HashToken("ret-tok"),
		DeletionTokenHash:  crypto.HashToken("del-tok"),
		Nonce:              "nonce",
		SecretType:         "file",
		StorageKey:         nil, // missing!
		BurnAfterRead:      false,
		ExpiresAt:          time.Now().Add(time.Hour),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/no-key/file", nil)
	req = withChiURLParam(req, "publicID", "no-key")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestDownloadFile_S3GetError(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	fs.getErr = fmt.Errorf("S3 not reachable")
	h := NewFileHandler(repo, fs, 100*1024*1024, nil)
	seedFileSecret(repo, fs, "s3-err", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/s3-err/file", nil)
	req = withChiURLParam(req, "publicID", "s3-err")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	rec := httptest.NewRecorder()

	h.DownloadFile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestDeleteSecret_TextSecretSkipsS3(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, nil)
	seedSecret(repo, "text-del", "ret-tok", "del-tok", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/text-del", nil)
	req = withChiURLParam(req, "publicID", "text-del")
	req.Header.Set("X-Retrieval-Token", "ret-tok")
	req.Header.Set("X-Deletion-Token", "del-tok")
	rec := httptest.NewRecorder()

	h.DeleteSecret(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	// No S3 objects should exist (none were created)
	if len(fs.objects) != 0 {
		t.Errorf("expected no S3 objects, got %d", len(fs.objects))
	}
}
