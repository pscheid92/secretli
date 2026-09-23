package cleanup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

func testMetrics() *metrics.SecretMetrics {
	return metrics.NewSecretMetrics(prometheus.NewRegistry())
}

// --- Mock implementations ---

// mockSecretRepo keeps a backlog like the database does: each batch takes up
// to limit rows from the front, and rows whose callback fails stay due.
type mockSecretRepo struct {
	expiredKeys         []string
	deleteExpiredErr    error
	deleteExpiredErrAt  int32 // fail on this call (1-based); 0 means every call
	deleteExpiredCalled atomic.Int32

	expiredUploads   []domain.UploadSession
	expiredUploadErr error
	abortCalls       int

	finishedBefore time.Time
}

func (m *mockSecretRepo) DeleteExpired(_ context.Context, _ time.Time, limit int, beforeDelete func(string) error) (domain.CleanupBatch, error) {
	call := m.deleteExpiredCalled.Add(1)
	if m.deleteExpiredErr != nil && (m.deleteExpiredErrAt == 0 || m.deleteExpiredErrAt == call) {
		return domain.CleanupBatch{}, m.deleteExpiredErr
	}
	batch := m.expiredKeys[:min(limit, len(m.expiredKeys))]
	var kept []string
	for _, key := range batch {
		if err := beforeDelete(key); err != nil {
			kept = append(kept, key)
		}
	}
	m.expiredKeys = append(kept, m.expiredKeys[len(batch):]...)
	return domain.CleanupBatch{Found: len(batch), Removed: len(batch) - len(kept)}, nil
}

func (m *mockSecretRepo) DeleteExpiredRetrievalSessions(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockSecretRepo) AbortExpiredUploadSessions(_ context.Context, _ time.Time, limit int, beforeAbort func(*domain.UploadSession) error) (domain.CleanupBatch, error) {
	m.abortCalls++
	if m.expiredUploadErr != nil {
		return domain.CleanupBatch{}, m.expiredUploadErr
	}
	batch := m.expiredUploads[:min(limit, len(m.expiredUploads))]
	var kept []domain.UploadSession
	for _, session := range batch {
		if err := beforeAbort(&session); err != nil {
			kept = append(kept, session)
		}
	}
	m.expiredUploads = append(kept, m.expiredUploads[len(batch):]...)
	return domain.CleanupBatch{Found: len(batch), Removed: len(batch) - len(kept)}, nil
}

func (m *mockSecretRepo) DeleteFinishedUploadSessions(_ context.Context, finishedBefore time.Time) (int64, error) {
	m.finishedBefore = finishedBefore
	return 0, nil
}

type mockFileStore struct {
	deletedKeys    []string
	deleteErr      error
	failKeys       map[string]bool
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
	if m.failKeys[key] {
		return errors.New("storage rejected delete")
	}
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
		expiredKeys: []string{"blobs/s1", "blobs/s2", "secrets/legacy"},
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

func TestRunCycle_WorksOffBacklogInBatches(t *testing.T) {
	repo := &mockSecretRepo{expiredKeys: keys("blobs/s", 2*batchSize+50)}
	store := &mockFileStore{}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	if got := repo.deleteExpiredCalled.Load(); got != 3 {
		t.Errorf("batches = %d, want 3", got)
	}
	if len(repo.expiredKeys) != 0 {
		t.Errorf("%d secrets left, want the whole backlog deleted", len(repo.expiredKeys))
	}
}

func TestRunCycle_FailedRowsDoNotStallTheRest(t *testing.T) {
	backlog := keys("blobs/s", 2*batchSize+50)
	repo := &mockSecretRepo{expiredKeys: backlog}
	store := &mockFileStore{failKeys: map[string]bool{backlog[4]: true}}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	// The failing row is retried by every batch; everything else goes.
	if len(repo.expiredKeys) != 1 || repo.expiredKeys[0] != backlog[4] {
		t.Errorf("left = %v, want only the failing secret", repo.expiredKeys)
	}
}

func TestRunCycle_StopsWhenNothingCanBeRemoved(t *testing.T) {
	repo := &mockSecretRepo{expiredKeys: keys("blobs/s", 2*batchSize)}
	store := &mockFileStore{deleteErr: errors.New("storage down")}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	if got := repo.deleteExpiredCalled.Load(); got != 1 {
		t.Errorf("batches = %d, want 1: retrying a failing store within the cycle is pointless", got)
	}
	if len(repo.expiredKeys) != 2*batchSize {
		t.Errorf("%d secrets left, want all kept for the next cycle", len(repo.expiredKeys))
	}
}

func TestRunCycle_KeepsCommittedBatchesAfterAnError(t *testing.T) {
	repo := &mockSecretRepo{
		expiredKeys:        keys("blobs/s", 2*batchSize),
		deleteExpiredErr:   errors.New("db connection lost"),
		deleteExpiredErrAt: 2,
	}
	m := testMetrics()

	w := NewWorker(time.Minute, repo, &mockFileStore{}, m)
	w.runCycle(context.Background())

	if len(repo.expiredKeys) != batchSize {
		t.Errorf("%d secrets left, want the first batch deleted before the error", len(repo.expiredKeys))
	}
	if got := counterValue(t, m.SecretsDeleted.WithLabelValues("cleanup")); got != batchSize {
		t.Errorf("deleted metric = %v, want %d", got, batchSize)
	}
	if got := counterValue(t, m.CleanupErrors); got != 1 {
		t.Errorf("cleanup errors = %v, want 1", got)
	}
}

func TestRunCycle_AbortsExpiredUploadsInBatches(t *testing.T) {
	uploads := make([]domain.UploadSession, batchSize+20)
	for i := range uploads {
		id := fmt.Sprintf("s%d", i)
		uploads[i] = domain.UploadSession{SessionID: id, StorageKey: "blobs/" + id, S3UploadID: "u" + id}
	}
	repo := &mockSecretRepo{expiredUploads: uploads}
	store := &mockFileStore{}

	w := NewWorker(time.Minute, repo, store, testMetrics())
	w.runCycle(context.Background())

	if repo.abortCalls != 2 || len(repo.expiredUploads) != 0 {
		t.Errorf("batches = %d, left = %d; want 2 batches and none left", repo.abortCalls, len(repo.expiredUploads))
	}
	if len(store.abortedUploads) != batchSize+20 {
		t.Errorf("aborted uploads = %d, want %d", len(store.abortedUploads), batchSize+20)
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

func keys(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return out
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var metric dto.Metric
	if err := c.Write(&metric); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return metric.GetCounter().GetValue()
}
