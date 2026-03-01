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

type mockSessionRepo struct {
	deleteExpiredCount  int64
	deleteExpiredErr    error
	deleteExpiredCalled atomic.Int32
}

func (m *mockSessionRepo) Create(_ context.Context, _ int64) (string, error) { return "", nil }
func (m *mockSessionRepo) GetByIDWithUser(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}
func (m *mockSessionRepo) Delete(_ context.Context, _ string) error { return nil }
func (m *mockSessionRepo) DeleteExpiredSessions(_ context.Context) (int64, error) {
	m.deleteExpiredCalled.Add(1)
	return m.deleteExpiredCount, m.deleteExpiredErr
}

type mockFileStore struct {
	deletedKeys []string
	deleteErr   error
	deleteCalled atomic.Int32
}

func (m *mockFileStore) Put(_ context.Context, _ string, _ io.Reader, _ int64) error { return nil }
func (m *mockFileStore) Get(_ context.Context, _ string) (io.ReadCloser, error)      { return nil, nil }
func (m *mockFileStore) Delete(_ context.Context, key string) error {
	m.deleteCalled.Add(1)
	m.deletedKeys = append(m.deletedKeys, key)
	return m.deleteErr
}

type mockRateLimiter struct {
	cleanupCalled atomic.Int32
	lastMaxAge    time.Duration
}

func (m *mockRateLimiter) CleanupStaleEntries(maxAge time.Duration) {
	m.cleanupCalled.Add(1)
	m.lastMaxAge = maxAge
}

// --- Tests ---

func TestRunCycle_Success(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredCount: 3,
		deleteExpiredKeys:  []string{"key1", "key2", "key3"},
	}
	sessionRepo := &mockSessionRepo{
		deleteExpiredCount: 2,
	}
	fileStore := &mockFileStore{}
	rateLimiter := &mockRateLimiter{}

	w := NewWorker(time.Minute, secretRepo, sessionRepo, fileStore, rateLimiter)
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
	if sessionRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpiredSessions called once, got %d", sessionRepo.deleteExpiredCalled.Load())
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
	if rateLimiter.cleanupCalled.Load() != 1 {
		t.Errorf("expected CleanupStaleEntries called once, got %d", rateLimiter.cleanupCalled.Load())
	}
	if rateLimiter.lastMaxAge != 10*time.Minute {
		t.Errorf("expected maxAge 10m, got %v", rateLimiter.lastMaxAge)
	}
}

func TestRunCycle_RepoErrors(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredErr: errors.New("db connection lost"),
	}
	sessionRepo := &mockSessionRepo{
		deleteExpiredErr: errors.New("session cleanup failed"),
	}
	rateLimiter := &mockRateLimiter{}

	w := NewWorker(time.Minute, secretRepo, sessionRepo, nil, rateLimiter)

	// Should not panic despite repo errors.
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
	if sessionRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpiredSessions called once, got %d", sessionRepo.deleteExpiredCalled.Load())
	}
	// Rate limiter should still be called even if repos error.
	if rateLimiter.cleanupCalled.Load() != 1 {
		t.Errorf("expected CleanupStaleEntries called once, got %d", rateLimiter.cleanupCalled.Load())
	}
}

func TestRunCycle_NilFileStore(t *testing.T) {
	secretRepo := &mockSecretRepo{
		deleteExpiredCount: 2,
		deleteExpiredKeys:  []string{"key1", "key2"},
	}
	sessionRepo := &mockSessionRepo{
		deleteExpiredCount: 1,
	}
	rateLimiter := &mockRateLimiter{}

	// fileStore is nil — should not panic when storageKeys are returned.
	w := NewWorker(time.Minute, secretRepo, sessionRepo, nil, rateLimiter)
	w.runCycle(context.Background())

	if secretRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpired called once, got %d", secretRepo.deleteExpiredCalled.Load())
	}
	if sessionRepo.deleteExpiredCalled.Load() != 1 {
		t.Errorf("expected DeleteExpiredSessions called once, got %d", sessionRepo.deleteExpiredCalled.Load())
	}
	if rateLimiter.cleanupCalled.Load() != 1 {
		t.Errorf("expected CleanupStaleEntries called once, got %d", rateLimiter.cleanupCalled.Load())
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	secretRepo := &mockSecretRepo{}
	sessionRepo := &mockSessionRepo{}

	w := NewWorker(10*time.Millisecond, secretRepo, sessionRepo, nil, nil)

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
