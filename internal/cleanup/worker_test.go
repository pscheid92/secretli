package cleanup

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

func testMetrics() *metrics.SecretMetrics {
	return metrics.NewSecretMetrics(prometheus.NewRegistry())
}

// --- Mock implementations ---

type mockSecretRepo struct {
	deleteExpiredKeys   []string
	deleteExpiredErr    error
	deleteExpiredCalled atomic.Int32

	expiredUploads   []expiredUploadSession
	expiredUploadErr error
}

type expiredUploadSession struct {
	session      domain.UploadSession
	secretExists bool
}

func (m *mockSecretRepo) DeleteExpired(_ context.Context, _ time.Time, beforeDelete func(string) error) (int64, error) {
	m.deleteExpiredCalled.Add(1)
	if m.deleteExpiredErr != nil {
		return 0, m.deleteExpiredErr
	}
	var deleted int64
	for _, id := range m.deleteExpiredKeys {
		if err := beforeDelete(id); err != nil {
			continue
		}
		deleted++
	}
	return deleted, nil
}

func (m *mockSecretRepo) DeleteExpiredRetrievalSessions(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockSecretRepo) AbortExpiredUploadSessions(_ context.Context, _ time.Time, beforeAbort func(*domain.UploadSession, bool) error) (int64, error) {
	if m.expiredUploadErr != nil {
		return 0, m.expiredUploadErr
	}
	var aborted int64
	for _, e := range m.expiredUploads {
		session := e.session
		if err := beforeAbort(&session, e.secretExists); err != nil {
			continue
		}
		aborted++
	}
	return aborted, nil
}

type mockFileStore struct {
	deletedKeys    []string
	deleteErr      error
	deleteCalled   atomic.Int32
	abortedUploads []string
	abortErr       error
}

func (m *mockFileStore) GetRange(_ context.Context, _ string, _, _ int64) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockFileStore) Delete(_ context.Context, key string) error {
	m.deleteCalled.Add(1)
	m.deletedKeys = append(m.deletedKeys, key)
	return m.deleteErr
}

func (m *mockFileStore) CreateMultipartUpload(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockFileStore) UploadPart(_ context.Context, _, _ string, _ int, _ io.Reader, _ int64) (string, error) {
	return "", nil
}

func (m *mockFileStore) CompleteMultipartUpload(_ context.Context, _, _ string, _ []domain.CompletedPart) error {
	return nil
}

func (m *mockFileStore) AbortMultipartUpload(_ context.Context, key, uploadID string) error {
	m.abortedUploads = append(m.abortedUploads, key+"#"+uploadID)
	return m.abortErr
}

// --- Tests ---

func TestRunCycle_ExpiredUploadSessions(t *testing.T) {
	repo := &mockSecretRepo{expiredUploads: []expiredUploadSession{
		{session: domain.UploadSession{SessionID: "s1", PublicID: "orphan", S3UploadID: "u1"}, secretExists: false},
		{session: domain.UploadSession{SessionID: "s2", PublicID: "owned", S3UploadID: "u2"}, secretExists: true},
	}}
	store := &mockFileStore{}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	if len(store.abortedUploads) != 2 || store.abortedUploads[0] != "secrets/orphan#u1" || store.abortedUploads[1] != "secrets/owned#u2" {
		t.Errorf("aborted uploads = %v, want both sessions aborted", store.abortedUploads)
	}
	// Only the session with no secret row has its (orphaned) object removed.
	if len(store.deletedKeys) != 1 || store.deletedKeys[0] != "secrets/orphan" {
		t.Errorf("deleted keys = %v, want only the orphaned object", store.deletedKeys)
	}
}

func TestRunCycle_ExpiredUploadSessionAbortFailureIsIsolated(t *testing.T) {
	repo := &mockSecretRepo{expiredUploads: []expiredUploadSession{
		{session: domain.UploadSession{SessionID: "s1", PublicID: "p1", S3UploadID: "u1"}},
	}}
	store := &mockFileStore{abortErr: errors.New("storage down")}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	if len(store.deletedKeys) != 0 {
		t.Errorf("object must not be deleted when the abort failed, got %v", store.deletedKeys)
	}
	if repo.deleteExpiredCalled.Load() != 1 {
		t.Error("secret cleanup should still run after upload cleanup errors")
	}
}

func TestRunCycle_Success(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredKeys: []string{"pub1", "pub2", "pub3"},
	}
	fileStore := &mockFileStore{}

	w := NewWorker(time.Minute, secretRepo, fileStore, testMetrics())
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
	if fileStore.deleteCalled.Load() != 3 {
		t.Errorf("expected fileStore.Delete called 3 times, got %d", fileStore.deleteCalled.Load())
	}
	if len(fileStore.deletedKeys) != 3 {
		t.Fatalf("expected 3 deleted keys, got %d", len(fileStore.deletedKeys))
	}
	for i, expected := range []string{"secrets/pub1", "secrets/pub2", "secrets/pub3"} {
		if fileStore.deletedKeys[i] != expected {
			t.Errorf("deletedKeys[%d] = %q, want %q", i, fileStore.deletedKeys[i], expected)
		}
	}
}

func TestRunCycle_RepoErrors(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredErr: errors.New("db connection lost"),
	}
	fileStore := &mockFileStore{}

	w := NewWorker(time.Minute, secretRepo, fileStore, testMetrics())

	// Should not panic despite repo errors.
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
}

func TestRunCycle_NoExpiredSecrets(t *testing.T) {
	secretRepo := &mockSecretRepo{}
	fileStore := &mockFileStore{}

	w := NewWorker(time.Minute, secretRepo, fileStore, testMetrics())
	w.runCycle(context.Background())

	if fileStore.deleteCalled.Load() != 0 {
		t.Errorf("expected no S3 deletions, got %d", fileStore.deleteCalled.Load())
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	secretRepo := &mockSecretRepo{}
	fileStore := &mockFileStore{}

	w := NewWorker(10*time.Millisecond, secretRepo, fileStore, testMetrics())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Let at least one tick fire.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Run returned as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
