package cleanup

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/model"
)

// --- Mock implementations ---

type mockSecretRepo struct {
	deleteExpiredCount  int64
	deleteExpiredKeys   []string
	deleteExpiredErr    error
	deleteExpiredCalled atomic.Int32
}

func (m *mockSecretRepo) Create(_ context.Context, _ *model.Secret) error { return nil }
func (m *mockSecretRepo) GetByPublicID(_ context.Context, _ string) (*model.Secret, error) {
	return nil, nil
}
func (m *mockSecretRepo) GetAndDeleteByPublicID(_ context.Context, _ string) (*model.Secret, error) {
	return nil, nil
}
func (m *mockSecretRepo) SetRetrievedAt(_ context.Context, _ string) error { return nil }
func (m *mockSecretRepo) Delete(_ context.Context, _ string) error        { return nil }
func (m *mockSecretRepo) DeleteExpired(_ context.Context) (int64, []string, error) {
	m.deleteExpiredCalled.Add(1)
	return m.deleteExpiredCount, m.deleteExpiredKeys, m.deleteExpiredErr
}

type mockFileStore struct {
	deletedKeys  []string
	deleteErr    error
	deleteCalled atomic.Int32
}

func (m *mockFileStore) Put(_ context.Context, _ string, _ io.Reader, _ int64) error { return nil }
func (m *mockFileStore) Get(_ context.Context, _ string) (io.ReadCloser, error)      { return nil, nil }
func (m *mockFileStore) Delete(_ context.Context, key string) error {
	m.deleteCalled.Add(1)
	m.deletedKeys = append(m.deletedKeys, key)
	return m.deleteErr
}

// --- Tests ---

func TestRunCycle_Success(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredCount: 3,
		deleteExpiredKeys:  []string{"key1", "key2", "key3"},
	}
	fileStore := &mockFileStore{}

	w := NewWorker(time.Minute, secretRepo, fileStore)
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
	for i, expected := range []string{"key1", "key2", "key3"} {
		if fileStore.deletedKeys[i] != expected {
			t.Errorf("deletedKeys[%d] = %q, want %q", i, fileStore.deletedKeys[i], expected)
		}
	}
}

func TestRunCycle_RepoErrors(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredErr: errors.New("db connection lost"),
	}

	w := NewWorker(time.Minute, secretRepo, nil)

	// Should not panic despite repo errors.
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
}

func TestRunCycle_NilFileStore(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredCount: 2,
		deleteExpiredKeys:  []string{"key1", "key2"},
	}

	// fileStore is nil — should not panic when storageKeys are returned.
	w := NewWorker(time.Minute, secretRepo, nil)
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	secretRepo := &mockSecretRepo{}

	w := NewWorker(10*time.Millisecond, secretRepo, nil)

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
