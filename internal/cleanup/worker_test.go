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
	objects             []domain.SecretObject
	deleteExpiredErr    error
	deleteExpiredCalled atomic.Int32
}

func (m *mockSecretRepo) Create(_ context.Context, _ *domain.Secret) error       { return nil }
func (m *mockSecretRepo) CreateUpload(_ context.Context, _ *domain.Secret) error { return nil }
func (m *mockSecretRepo) GetByPublicID(_ context.Context, _ string) (*domain.Secret, error) {
	return nil, nil
}
func (m *mockSecretRepo) GetPendingUploadByPublicID(_ context.Context, _ string) (*domain.Secret, error) {
	return nil, nil
}
func (m *mockSecretRepo) ClaimBurnAfterRead(_ context.Context, _, _ string) error { return nil }
func (m *mockSecretRepo) StartRetrievalSession(_ context.Context, _, _, _ string, _ time.Time) (*domain.Secret, error) {
	return nil, nil
}
func (m *mockSecretRepo) GetByRetrievalSession(_ context.Context, _, _ string) (*domain.Secret, error) {
	return nil, nil
}
func (m *mockSecretRepo) GetObject(_ context.Context, _, _ string, _ int32) (*domain.SecretObject, error) {
	return nil, nil
}
func (m *mockSecretRepo) CreateObject(_ context.Context, _ *domain.SecretObject) error { return nil }
func (m *mockSecretRepo) ListObjects(_ context.Context, publicID string) ([]domain.SecretObject, error) {
	objects := make([]domain.SecretObject, 0)
	for _, object := range m.objects {
		if object.PublicID == publicID {
			objects = append(objects, object)
		}
	}
	return objects, nil
}
func (m *mockSecretRepo) CompleteUpload(_ context.Context, _ string, _ int64, _ time.Time) error {
	return nil
}
func (m *mockSecretRepo) Delete(_ context.Context, _ string) error { return nil }
func (m *mockSecretRepo) DeleteExpired(_ context.Context, beforeDelete func(string, []domain.SecretObject) error) (int64, error) {
	m.deleteExpiredCalled.Add(1)
	if m.deleteExpiredErr != nil {
		return 0, m.deleteExpiredErr
	}
	var deleted int64
	for _, id := range m.deleteExpiredKeys {
		objects := make([]domain.SecretObject, 0)
		for _, object := range m.objects {
			if object.PublicID == id {
				objects = append(objects, object)
			}
		}
		if err := beforeDelete(id, objects); err != nil {
			continue
		}
		deleted++
	}
	return deleted, nil
}
func (m *mockSecretRepo) DeleteExpiredRetrievalSessions(_ context.Context) (int64, error) {
	return 0, nil
}

type mockFileStore struct {
	deletedKeys  []string
	deleteErr    error
	deleteCalled atomic.Int32
}

func (m *mockFileStore) Put(_ context.Context, _ string, _ io.Reader, _ int64) error { return nil }
func (m *mockFileStore) Get(_ context.Context, _ string) (io.ReadCloser, error)      { return nil, nil }
func (m *mockFileStore) GetRange(_ context.Context, _ string, _, _ int64) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockFileStore) Delete(_ context.Context, key string) error {
	m.deleteCalled.Add(1)
	m.deletedKeys = append(m.deletedKeys, key)
	return m.deleteErr
}

// --- Tests ---

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

func TestRunCycle_DeletesChunkedObjects(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredKeys: []string{"chunked"},
		objects: []domain.SecretObject{
			{PublicID: "chunked", ObjectKind: domain.ObjectKindManifest, ObjectIndex: domain.ManifestObjectIndex},
			{PublicID: "chunked", ObjectKind: domain.ObjectKindChunk, ObjectIndex: 0},
			{PublicID: "chunked", ObjectKind: domain.ObjectKindChunk, ObjectIndex: 1},
		},
	}
	fileStore := &mockFileStore{}

	w := NewWorker(time.Minute, secretRepo, fileStore, testMetrics())
	w.runCycle(context.Background())

	expected := []string{
		"secrets/chunked",
		"secrets/chunked/manifest",
		"secrets/chunked/chunks/0",
		"secrets/chunked/chunks/1",
	}
	if len(fileStore.deletedKeys) != len(expected) {
		t.Fatalf("deleted keys = %v, want %v", fileStore.deletedKeys, expected)
	}
	for i, key := range expected {
		if fileStore.deletedKeys[i] != key {
			t.Fatalf("deletedKeys[%d] = %q, want %q", i, fileStore.deletedKeys[i], key)
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
