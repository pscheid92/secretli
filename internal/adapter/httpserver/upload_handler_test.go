package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
)

type uploadMockRepo struct {
	// mu stands in for the session row lock: CompleteUploadSession holds it
	// while finalize runs.
	mu        sync.Mutex
	sessions  map[string]*domain.UploadSession
	parts     map[string]map[int]domain.UploadPart
	secrets   map[string]*domain.Secret
	insertErr error
}

func newUploadMockRepo() *uploadMockRepo {
	return &uploadMockRepo{
		sessions: make(map[string]*domain.UploadSession),
		parts:    make(map[string]map[int]domain.UploadPart),
		secrets:  make(map[string]*domain.Secret),
	}
}

func (m *uploadMockRepo) CreateUploadSession(_ context.Context, session *domain.UploadSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[session.SessionID]; ok {
		return domain.ErrDuplicate
	}
	copy := *session
	m.sessions[session.SessionID] = &copy
	return nil
}

func (m *uploadMockRepo) GetUploadSession(_ context.Context, sessionID string) (*domain.UploadSession, []domain.UploadPart, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil, domain.ErrNotFound
	}
	partsByNumber := m.parts[sessionID]
	parts := make([]domain.UploadPart, 0, len(partsByNumber))
	for _, part := range partsByNumber {
		parts = append(parts, part)
	}
	copy := *session
	return &copy, parts, nil
}

func (m *uploadMockRepo) RecordUploadPart(_ context.Context, part *domain.UploadPart) (*domain.UploadPart, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.parts[part.SessionID] == nil {
		m.parts[part.SessionID] = make(map[int]domain.UploadPart)
	}
	if existing, ok := m.parts[part.SessionID][part.PartNumber]; ok {
		if existing.Offset != part.Offset || existing.Size != part.Size || existing.SHA256 != part.SHA256 {
			return nil, domain.ErrConflict
		}
		return &existing, nil
	}
	copy := *part
	m.parts[part.SessionID][part.PartNumber] = copy
	return &copy, nil
}

func (m *uploadMockRepo) CompleteUploadSession(_ context.Context, sessionID string, now time.Time, finalize func(*domain.UploadSession, []domain.UploadPart) error) (*domain.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	switch session.State {
	case domain.UploadSessionStateCompleted:
		copy := *session
		return &copy, nil
	case domain.UploadSessionStatePending:
	default:
		return nil, domain.ErrConflict
	}

	parts := make([]domain.UploadPart, 0, len(m.parts[sessionID]))
	for _, part := range m.parts[sessionID] {
		parts = append(parts, part)
	}
	locked := *session
	if err := finalize(&locked, parts); err != nil {
		return nil, err
	}
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	if _, exists := m.secrets[session.PublicID]; exists {
		return nil, domain.ErrDuplicate
	}

	m.secrets[session.PublicID] = &domain.Secret{
		PublicID:   session.PublicID,
		StorageKey: session.StorageKey,
		BlobSize:   session.BlobSize,
		ExpiresAt:  session.SecretExpiresAt,
	}
	session.State = domain.UploadSessionStateCompleted
	session.CompletedAt = &now
	session.EncryptedMeta = ""
	delete(m.parts, sessionID)
	copy := *session
	return &copy, nil
}

func (m *uploadMockRepo) AbortUploadSession(_ context.Context, sessionID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return domain.ErrNotFound
	}
	if session.State != domain.UploadSessionStatePending {
		return domain.ErrConflict
	}
	session.State = domain.UploadSessionStateAborted
	return nil
}

func (m *uploadMockRepo) ClearUploadParts(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.parts, sessionID)
	return nil
}

type uploadMockStore struct {
	uploadID       string
	uploadedParts  map[int][]byte
	completedParts []domain.CompletedPart
	completeCalls  int
	completeErr    error
	aborted        bool
	abortErr       error
	deleted        []string
}

func newUploadMockStore() *uploadMockStore {
	return &uploadMockStore{uploadID: "s3-upload-id", uploadedParts: make(map[int][]byte)}
}

func (m *uploadMockStore) Put(_ context.Context, _ string, _ io.Reader, _ int64) error { return nil }
func (m *uploadMockStore) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (m *uploadMockStore) GetRange(_ context.Context, _ string, _, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (m *uploadMockStore) Delete(_ context.Context, key string) error {
	m.deleted = append(m.deleted, key)
	return nil
}
func (m *uploadMockStore) CreateMultipartUpload(_ context.Context, _ string) (string, error) {
	return m.uploadID, nil
}
func (m *uploadMockStore) UploadPart(_ context.Context, _ string, _ string, partNumber int, reader io.Reader, _ int64) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	m.uploadedParts[partNumber] = data
	return fmt.Sprintf("etag-%d", partNumber), nil
}
func (m *uploadMockStore) CompleteMultipartUpload(_ context.Context, _ string, _ string, parts []domain.CompletedPart) error {
	m.completeCalls++
	if m.completeErr != nil {
		return m.completeErr
	}
	m.completedParts = append([]domain.CompletedPart(nil), parts...)
	return nil
}
func (m *uploadMockStore) AbortMultipartUpload(_ context.Context, _ string, _ string) error {
	if m.abortErr != nil {
		return m.abortErr
	}
	m.aborted = true
	return nil
}

func TestUploadSession_CreateSuccess(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	req := createUploadSessionHTTPRequest(t, map[string]any{
		"public_id":       testPublicID("multipart-create"),
		"metadata_token":  testToken("multipart metadata"),
		"blob_token":      testToken("multipart blob"),
		"deletion_token":  testToken("multipart deletion"),
		"encrypted_meta":  testEncryptedMeta(),
		"expiration":      "1d",
		"burn_after_read": false,
		"blob_size":       10 * 1024 * 1024,
	})
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)

	callHandler(c, h.CreateUploadSession)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var body struct {
		SessionID   string `json:"session_id"`
		UploadToken string `json:"upload_token"`
		PartSize    int64  `json:"part_size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.SessionID == "" || body.UploadToken == "" {
		t.Fatal("response missing session credentials")
	}
	if body.PartSize != multipartUploadPartSize {
		t.Errorf("part_size = %d, want %d", body.PartSize, multipartUploadPartSize)
	}
	if len(repo.secrets) != 0 {
		t.Fatal("create upload session should not create active secret")
	}
	if got, want := repo.sessions[body.SessionID].StorageKey, domain.UploadStorageKey(body.SessionID); got != want {
		t.Errorf("storage key = %q, want per-session key %q", got, want)
	}
}

func TestUploadPart_IdempotentAndConflict(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("upload token")
	session := seedUploadSession(repo, uploadToken, 6)
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())
	payload := []byte("abcdef")
	hash := sha256HexTest(payload)

	req := uploadPartRequest(session.SessionID, uploadToken, 1, 0, payload, hash)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("sessionID", "partNumber")
	c.SetParamValues(session.SessionID, "1")
	callHandler(c, h.UploadPart)
	if rec.Code != http.StatusOK {
		t.Fatalf("first upload status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := string(store.uploadedParts[1]); got != string(payload) {
		t.Fatalf("uploaded part = %q, want %q", got, string(payload))
	}

	req = uploadPartRequest(session.SessionID, uploadToken, 1, 0, payload, hash)
	rec = httptest.NewRecorder()
	c = newEchoContext(req, rec)
	c.SetParamNames("sessionID", "partNumber")
	c.SetParamValues(session.SessionID, "1")
	callHandler(c, h.UploadPart)
	if rec.Code != http.StatusOK {
		t.Fatalf("idempotent upload status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	req = uploadPartRequest(session.SessionID, uploadToken, 1, 0, []byte("zzzzzz"), sha256HexTest([]byte("zzzzzz")))
	rec = httptest.NewRecorder()
	c = newEchoContext(req, rec)
	c.SetParamNames("sessionID", "partNumber")
	c.SetParamValues(session.SessionID, "1")
	callHandler(c, h.UploadPart)
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, want %d. body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestUploadPart_RejectsHashMismatchBeforeS3Upload(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("upload hash mismatch token")
	session := seedUploadSession(repo, uploadToken, 6)
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	req := uploadPartRequest(session.SessionID, uploadToken, 1, 0, []byte("abcdef"), sha256HexTest([]byte("zzzzzz")))
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("sessionID", "partNumber")
	c.SetParamValues(session.SessionID, "1")
	callHandler(c, h.UploadPart)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := len(store.uploadedParts); got != 0 {
		t.Fatalf("uploaded parts = %d, want 0", got)
	}
}

func TestCompleteUploadSession_FailsWhenPartMissing(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete missing token")
	session := seedUploadSession(repo, uploadToken, s3MinimumPartSize+3)
	repo.parts[session.SessionID] = map[int]domain.UploadPart{
		1: {SessionID: session.SessionID, PartNumber: 1, Offset: 0, Size: s3MinimumPartSize, SHA256: sha256HexTest([]byte("a")), ETag: "etag-1"},
	}
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	req := completeUploadRequest(session.SessionID, uploadToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("sessionID")
	c.SetParamValues(session.SessionID)
	callHandler(c, h.CompleteUploadSession)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if len(store.completedParts) != 0 {
		t.Fatal("S3 complete should not be called for missing parts")
	}
}

func TestCompleteUploadSession_CreatesSecret(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete token")
	session := seedUploadSession(repo, uploadToken, s3MinimumPartSize+3)
	repo.parts[session.SessionID] = map[int]domain.UploadPart{
		1: {SessionID: session.SessionID, PartNumber: 1, Offset: 0, Size: s3MinimumPartSize, SHA256: sha256HexTest([]byte("a")), ETag: "etag-1"},
		2: {SessionID: session.SessionID, PartNumber: 2, Offset: s3MinimumPartSize, Size: 3, SHA256: sha256HexTest([]byte("b")), ETag: "etag-2"},
	}
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	req := completeUploadRequest(session.SessionID, uploadToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("sessionID")
	c.SetParamValues(session.SessionID)
	callHandler(c, h.CompleteUploadSession)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if _, ok := repo.secrets[session.PublicID]; !ok {
		t.Fatal("completed upload should create active secret")
	}
	if got := len(store.completedParts); got != 2 {
		t.Fatalf("completed parts = %d, want 2", got)
	}
}

func TestUploadPart_RejectsPartsOutsideDeclaredBlob(t *testing.T) {
	payload := []byte("abcdef")
	hash := sha256HexTest(payload)
	tests := []struct {
		name       string
		blobSize   int64
		partNumber string
		offset     int64
	}{
		{name: "part number beyond part count", blobSize: 6, partNumber: "2", offset: 0},
		{name: "offset below lower bound for part number", blobSize: 2 * s3MinimumPartSize, partNumber: "2", offset: 0},
		{name: "part overruns blob", blobSize: 4, partNumber: "1", offset: 0},
		{name: "non-final part below minimum size", blobSize: 12, partNumber: "1", offset: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newUploadMockRepo()
			store := newUploadMockStore()
			uploadToken := testToken("bounds " + tt.name)
			session := seedUploadSession(repo, uploadToken, tt.blobSize)
			h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

			req := uploadPartRequest(session.SessionID, uploadToken, 1, tt.offset, payload, hash)
			rec := httptest.NewRecorder()
			c := newEchoContext(req, rec)
			c.SetParamNames("sessionID", "partNumber")
			c.SetParamValues(session.SessionID, tt.partNumber)
			callHandler(c, h.UploadPart)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			if len(store.uploadedParts) != 0 {
				t.Fatal("part must be rejected before reaching S3")
			}
		})
	}
}

func TestValidatePartPlacement(t *testing.T) {
	const mib = 1024 * 1024
	tests := []struct {
		name       string
		blobSize   int64
		partNumber int
		offset     int64
		size       int64
		wantErr    bool
	}{
		{name: "single tiny final part", blobSize: 6, partNumber: 1, offset: 0, size: 6},
		{name: "first full part", blobSize: 40 * mib, partNumber: 1, offset: 0, size: 28 * mib},
		{name: "final short part", blobSize: 40 * mib, partNumber: 2, offset: 28 * mib, size: 12 * mib},
		{name: "part number too high", blobSize: 6 * mib, partNumber: 3, offset: 5 * mib, size: mib, wantErr: true},
		{name: "offset too small for part number", blobSize: 40 * mib, partNumber: 3, offset: 6 * mib, size: 5 * mib, wantErr: true},
		{name: "overruns blob", blobSize: 40 * mib, partNumber: 2, offset: 28 * mib, size: 13 * mib, wantErr: true},
		{name: "undersized non-final part", blobSize: 40 * mib, partNumber: 1, offset: 0, size: 4 * mib, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePartPlacement(tt.blobSize, tt.partNumber, tt.offset, tt.size)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePartPlacement() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompleteUploadSession_DBFailureDiscardsOwnObject(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete db failure token")
	session := seedSinglePartUploadSession(repo, uploadToken)
	repo.insertErr = errors.New("database down")
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	rec := callComplete(h, session.SessionID, uploadToken)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != session.StorageKey {
		t.Fatalf("completed object should be deleted after DB failure, deleted = %v", store.deleted)
	}
	if repo.sessions[session.SessionID].State != domain.UploadSessionStateAborted {
		t.Fatal("session should be aborted before its object is deleted")
	}
}

func TestCompleteUploadSession_RepeatedCompleteIsIdempotent(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete twice token")
	session := seedSinglePartUploadSession(repo, uploadToken)
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	first := callComplete(h, session.SessionID, uploadToken)
	second := callComplete(h, session.SessionID, uploadToken)

	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("statuses = %d, %d; want both %d. second body: %s", first.Code, second.Code, http.StatusCreated, second.Body.String())
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("repeated complete answered differently: %s vs %s", first.Body.String(), second.Body.String())
	}
	if store.completeCalls != 1 {
		t.Errorf("storage complete calls = %d, want 1", store.completeCalls)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("a repeated complete must not delete the live object, deleted = %v", store.deleted)
	}
}

func TestCompleteUploadSession_ConcurrentCompletesKeepObject(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete concurrently token")
	session := seedSinglePartUploadSession(repo, uploadToken)
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	const requests = 8
	codes := make([]int, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Go(func() {
			codes[i] = callComplete(h, session.SessionID, uploadToken).Code
		})
	}
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusCreated {
			t.Errorf("request %d status = %d, want %d", i, code, http.StatusCreated)
		}
	}
	if store.completeCalls != 1 {
		t.Errorf("storage complete calls = %d, want 1", store.completeCalls)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("concurrent completes must not delete the live object, deleted = %v", store.deleted)
	}
	if _, ok := repo.secrets[session.PublicID]; !ok {
		t.Fatal("secret should exist")
	}
}

func TestCompleteUploadSession_DuplicatePublicIDDeletesOnlyOwnObject(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete duplicate token")
	session := seedSinglePartUploadSession(repo, uploadToken)
	// Another upload already turned this public_id into a secret.
	otherKey := domain.UploadStorageKey(testToken("other session"))
	repo.secrets[session.PublicID] = &domain.Secret{PublicID: session.PublicID, StorageKey: otherKey}
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	rec := callComplete(h, session.SessionID, uploadToken)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if len(store.deleted) != 1 || store.deleted[0] != session.StorageKey {
		t.Fatalf("deleted = %v, want only this session's object %q", store.deleted, session.StorageKey)
	}
	if repo.secrets[session.PublicID].StorageKey != otherKey {
		t.Fatal("the existing secret must be left alone")
	}
}

func TestCompleteUploadSession_AbortedSessionConflicts(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	uploadToken := testToken("complete aborted token")
	session := seedSinglePartUploadSession(repo, uploadToken)
	session.State = domain.UploadSessionStateAborted
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	rec := callComplete(h, session.SessionID, uploadToken)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if store.completeCalls != 0 || len(store.deleted) != 0 {
		t.Fatalf("aborted session must not touch storage: complete calls = %d, deleted = %v", store.completeCalls, store.deleted)
	}
}

func TestCompleteUploadSession_InvalidPartsResetsRecordedParts(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	store.completeErr = fmt.Errorf("wrapped: %w", domain.ErrInvalidParts)
	uploadToken := testToken("complete invalid parts token")
	session := seedUploadSession(repo, uploadToken, 3)
	repo.parts[session.SessionID] = map[int]domain.UploadPart{
		1: {SessionID: session.SessionID, PartNumber: 1, Offset: 0, Size: 3, SHA256: sha256HexTest([]byte("a")), ETag: "stale"},
	}
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	req := completeUploadRequest(session.SessionID, uploadToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("sessionID")
	c.SetParamValues(session.SessionID)
	callHandler(c, h.CompleteUploadSession)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if len(repo.parts[session.SessionID]) != 0 {
		t.Fatal("recorded parts should be cleared so the client can re-upload")
	}
	if repo.sessions[session.SessionID].State != domain.UploadSessionStatePending {
		t.Fatal("session should stay pending after an invalid-parts conflict")
	}
}

func TestCompleteUploadSession_UploadGoneAbortsSession(t *testing.T) {
	repo := newUploadMockRepo()
	store := newUploadMockStore()
	store.completeErr = fmt.Errorf("wrapped: %w", domain.ErrUploadNotFound)
	uploadToken := testToken("complete upload gone token")
	session := seedUploadSession(repo, uploadToken, 3)
	repo.parts[session.SessionID] = map[int]domain.UploadPart{
		1: {SessionID: session.SessionID, PartNumber: 1, Offset: 0, Size: 3, SHA256: sha256HexTest([]byte("a")), ETag: "etag-1"},
	}
	h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

	req := completeUploadRequest(session.SessionID, uploadToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("sessionID")
	c.SetParamValues(session.SessionID)
	callHandler(c, h.CompleteUploadSession)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if repo.sessions[session.SessionID].State != domain.UploadSessionStateAborted {
		t.Fatal("session should be aborted when storage no longer knows the upload")
	}
	if len(store.deleted) != 1 || store.deleted[0] != session.StorageKey {
		t.Fatalf("deleted = %v, want the session's own object removed", store.deleted)
	}
}

func TestAbortUploadSession(t *testing.T) {
	t.Run("aborts pending session", func(t *testing.T) {
		repo := newUploadMockRepo()
		store := newUploadMockStore()
		uploadToken := testToken("abort token")
		session := seedUploadSession(repo, uploadToken, 6)
		h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

		req := abortUploadRequest(session.SessionID, uploadToken)
		rec := httptest.NewRecorder()
		c := newEchoContext(req, rec)
		c.SetParamNames("sessionID")
		c.SetParamValues(session.SessionID)
		callHandler(c, h.AbortUploadSession)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
		}
		if !store.aborted {
			t.Fatal("S3 multipart upload should be aborted")
		}
		if repo.sessions[session.SessionID].State != domain.UploadSessionStateAborted {
			t.Fatal("session should be marked aborted")
		}
	})

	t.Run("abort is idempotent", func(t *testing.T) {
		repo := newUploadMockRepo()
		store := newUploadMockStore()
		uploadToken := testToken("abort twice token")
		session := seedUploadSession(repo, uploadToken, 6)
		session.State = domain.UploadSessionStateAborted
		h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

		req := abortUploadRequest(session.SessionID, uploadToken)
		rec := httptest.NewRecorder()
		c := newEchoContext(req, rec)
		c.SetParamNames("sessionID")
		c.SetParamValues(session.SessionID)
		callHandler(c, h.AbortUploadSession)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
		}
		if store.aborted {
			t.Fatal("S3 abort should not be called again for an aborted session")
		}
	})

	t.Run("rejects completed session", func(t *testing.T) {
		repo := newUploadMockRepo()
		store := newUploadMockStore()
		uploadToken := testToken("abort completed token")
		session := seedUploadSession(repo, uploadToken, 6)
		session.State = domain.UploadSessionStateCompleted
		h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

		req := abortUploadRequest(session.SessionID, uploadToken)
		rec := httptest.NewRecorder()
		c := newEchoContext(req, rec)
		c.SetParamNames("sessionID")
		c.SetParamValues(session.SessionID)
		callHandler(c, h.AbortUploadSession)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusConflict, rec.Body.String())
		}
	})

	t.Run("rejects wrong upload token", func(t *testing.T) {
		repo := newUploadMockRepo()
		store := newUploadMockStore()
		session := seedUploadSession(repo, testToken("abort right token"), 6)
		h := NewUploadHandler(repo, store, 100*1024*1024, testMetrics())

		req := abortUploadRequest(session.SessionID, testToken("abort wrong token"))
		rec := httptest.NewRecorder()
		c := newEchoContext(req, rec)
		c.SetParamNames("sessionID")
		c.SetParamValues(session.SessionID)
		callHandler(c, h.AbortUploadSession)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
		if store.aborted || repo.sessions[session.SessionID].State != domain.UploadSessionStatePending {
			t.Fatal("wrong token must not abort the session")
		}
	})
}

func abortUploadRequest(sessionID, uploadToken string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/uploads/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+uploadToken)
	return req
}

func seedUploadSession(repo *uploadMockRepo, uploadToken string, blobSize int64) *domain.UploadSession {
	session := &domain.UploadSession{
		SessionID:         testToken("session " + uploadToken),
		UploadTokenHash:   tokencrypto.TokenHash(uploadToken),
		PublicID:          testPublicID("session " + uploadToken),
		StorageKey:        domain.UploadStorageKey(testToken("session " + uploadToken)),
		S3UploadID:        "s3-upload-id",
		BlobSize:          blobSize,
		MetadataTokenHash: tokencrypto.TokenHash(testToken("meta " + uploadToken)),
		BlobTokenHash:     tokencrypto.TokenHash(testToken("blob " + uploadToken)),
		DeletionTokenHash: tokencrypto.TokenHash(testToken("delete " + uploadToken)),
		EncryptedMeta:     testEncryptedMeta(),
		BurnAfterRead:     false,
		SecretExpiresAt:   time.Now().Add(time.Hour),
		UploadExpiresAt:   time.Now().Add(time.Hour),
		State:             domain.UploadSessionStatePending,
	}
	repo.sessions[session.SessionID] = session
	return session
}

func seedSinglePartUploadSession(repo *uploadMockRepo, uploadToken string) *domain.UploadSession {
	session := seedUploadSession(repo, uploadToken, 3)
	repo.parts[session.SessionID] = map[int]domain.UploadPart{
		1: {SessionID: session.SessionID, PartNumber: 1, Offset: 0, Size: 3, SHA256: sha256HexTest([]byte("a")), ETag: "etag-1"},
	}
	return session
}

func callComplete(h *UploadHandler, sessionID, uploadToken string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c := newEchoContext(completeUploadRequest(sessionID, uploadToken), rec)
	c.SetParamNames("sessionID")
	c.SetParamValues(sessionID)
	callHandler(c, h.CompleteUploadSession)
	return rec
}

func createUploadSessionHTTPRequest(t *testing.T, body map[string]any) *http.Request {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/uploads", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func uploadPartRequest(sessionID, uploadToken string, partNumber int, offset int64, payload []byte, hash string) *http.Request {
	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/v1/secrets/uploads/%s/parts/%d", sessionID, partNumber),
		bytes.NewReader(payload),
	)
	req.Header.Set("Authorization", "Bearer "+uploadToken)
	req.Header.Set(HeaderPartOffset, strconvFormatInt(offset))
	req.Header.Set(HeaderPartSize, strconvFormatInt(int64(len(payload))))
	req.Header.Set(HeaderPartSHA256, hash)
	return req
}

func completeUploadRequest(sessionID, uploadToken string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/uploads/"+sessionID+"/complete", nil)
	req.Header.Set("Authorization", "Bearer "+uploadToken)
	return req
}

func sha256HexTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func strconvFormatInt(value int64) string {
	return fmt.Sprintf("%d", value)
}
