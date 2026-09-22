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

	expiredUploads   []domain.UploadSession
	expiredUploadErr error

	finishedBefore time.Time
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

func (m *mockSecretRepo) AbortExpiredUploadSessions(_ context.Context, _ time.Time, beforeAbort func(*domain.UploadSession) error) (int64, error) {
	if m.expiredUploadErr != nil {
		return 0, m.expiredUploadErr
	}
	var aborted int64
	for _, session := range m.expiredUploads {
		if err := beforeAbort(&session); err != nil {
			continue
		}
		aborted++
	}
	return aborted, nil
}

func (m *mockSecretRepo) DeleteFinishedUploadSessions(_ context.Context, finishedBefore time.Time) (int64, error) {
	m.finishedBefore = finishedBefore
	return 0, nil
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
	repo := &mockSecretRepo{expiredUploads: []domain.UploadSession{
		{SessionID: "s1", StorageKey: "blobs/s1", S3UploadID: "u1"},
		{SessionID: "s2", StorageKey: "blobs/s2", S3UploadID: "u2"},
	}}
	store := &mockFileStore{}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	if len(store.abortedUploads) != 2 || store.abortedUploads[0] != "blobs/s1#u1" || store.abortedUploads[1] != "blobs/s2#u2" {
		t.Errorf("aborted uploads = %v, want both sessions aborted", store.abortedUploads)
	}
	// Each session's key is its own, so any object left behind is removed.
	if len(store.deletedKeys) != 2 || store.deletedKeys[0] != "blobs/s1" || store.deletedKeys[1] != "blobs/s2" {
		t.Errorf("deleted keys = %v, want both sessions' objects", store.deletedKeys)
	}
}

func TestRunCycle_PurgesFinishedUploadSessions(t *testing.T) {
	repo := &mockSecretRepo{}
	w := NewWorker(time.Minute, repo, &mockFileStore{}, testMetrics())

	before := time.Now()
	w.runCycle(context.Background())
	after := time.Now()

	if repo.finishedBefore.Before(before.Add(-finishedUploadRetention)) || repo.finishedBefore.After(after.Add(-finishedUploadRetention)) {
		t.Errorf("finished sessions purged before %v, want %v before the cycle", repo.finishedBefore, finishedUploadRetention)
	}
}

func TestRunCycle_ExpiredUploadSessionAbortFailureIsIsolated(t *testing.T) {
	repo := &mockSecretRepo{expiredUploads: []domain.UploadSession{
		{SessionID: "s1", StorageKey: "blobs/s1", S3UploadID: "u1"},
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
		deleteExpiredKeys: []string{"blobs/s1", "blobs/s2", "secrets/legacy"},
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
	for i, expected := range []string{"blobs/s1", "blobs/s2", "secrets/legacy"} {
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
