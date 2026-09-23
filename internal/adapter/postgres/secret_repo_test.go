package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
)

// testBatchSize is larger than any test's backlog unless a test is about batching.
const testBatchSize = 100

func newTestSecret(publicID string, expiresAt time.Time) *domain.Secret {
	return &domain.Secret{
		PublicID:          publicID,
		MetadataTokenHash: tokencrypto.TokenHash("metadata-token-" + publicID),
		BlobTokenHash:     tokencrypto.TokenHash("blob-token-" + publicID),
		DeletionTokenHash: tokencrypto.TokenHash("deletion-token-" + publicID),
		EncryptedMeta:     "v2$nonce$meta-" + publicID,
		BlobSize:          1024,
		BurnAfterRead:     false,
		ExpiresAt:         expiresAt,
		StorageKey:        "secrets/" + publicID,
	}
}

func markRetrieved(t *testing.T, pool *pgxpool.Pool, publicID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE secrets SET retrieved_at = $2 WHERE public_id = $1", publicID, time.Now()); err != nil {
		t.Fatalf("mark retrieved %s: %v", publicID, err)
	}
}

func TestSecretRepo_CreateAndGet(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("pub-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	got, err := repo.GetByPublicID(ctx, "pub-001", time.Now())
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}

	if got.PublicID != "pub-001" {
		t.Errorf("public_id = %q, want %q", got.PublicID, "pub-001")
	}
	if got.MetadataTokenHash != tokencrypto.TokenHash("metadata-token-pub-001") {
		t.Errorf("metadata_token_hash = %q, want hash", got.MetadataTokenHash)
	}
	if got.BlobTokenHash != tokencrypto.TokenHash("blob-token-pub-001") {
		t.Errorf("blob_token_hash = %q, want hash", got.BlobTokenHash)
	}
	if got.DeletionTokenHash != tokencrypto.TokenHash("deletion-token-pub-001") {
		t.Errorf("deletion_token_hash = %q, want hash", got.DeletionTokenHash)
	}
	if got.EncryptedMeta != "v2$nonce$meta-pub-001" {
		t.Errorf("encrypted_meta = %q, want %q", got.EncryptedMeta, "v2$nonce$meta-pub-001")
	}
	if got.BlobSize != 1024 {
		t.Errorf("blob_size = %d, want %d", got.BlobSize, 1024)
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestSecretRepo_CreateDuplicate(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("dup-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create first: %v", err)
	}

	secret2 := newTestSecret("dup-001", time.Now().Add(2*time.Hour))
	err := repo.Create(ctx, secret2, time.Now())
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestSecretRepo_GetExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("expired-001", time.Now().Add(-1*time.Hour))
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create expired secret: %v", err)
	}

	_, err := repo.GetByPublicID(ctx, "expired-001", time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired secret, got %v", err)
	}
}

func TestSecretRepo_RetrievalSession(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("session-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create: %v", err)
	}

	sessionHash := tokencrypto.TokenHash("session-token")
	got, err := repo.StartRetrievalSession(
		ctx,
		"session-001",
		tokencrypto.TokenHash("blob-token-session-001"),
		sessionHash,
		time.Now().Add(15*time.Minute),
		time.Now(),
	)
	if err != nil {
		t.Fatalf("start retrieval session: %v", err)
	}
	if got.PublicID != "session-001" {
		t.Errorf("public_id = %q, want %q", got.PublicID, "session-001")
	}

	got, err = repo.GetByRetrievalSession(ctx, "session-001", sessionHash, time.Now())
	if err != nil {
		t.Fatalf("get by retrieval session: %v", err)
	}
	if got.PublicID != "session-001" {
		t.Errorf("session public_id = %q, want %q", got.PublicID, "session-001")
	}

	_, err = repo.GetByRetrievalSession(ctx, "session-001", tokencrypto.TokenHash("wrong-session"), time.Now())
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for wrong session, got %v", err)
	}

	_, err = repo.StartRetrievalSession(
		ctx,
		"session-001",
		tokencrypto.TokenHash("wrong-blob-token"),
		tokencrypto.TokenHash("session-token-wrong-blob"),
		time.Now().Add(15*time.Minute),
		time.Now(),
	)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for wrong blob token, got %v", err)
	}
}

func TestSecretRepo_RetrievalSessionExpiry(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	active := newTestSecret("session-expiry-active", time.Now().Add(1*time.Hour))
	expired := newTestSecret("session-expiry-expired", time.Now().Add(1*time.Hour))
	for _, secret := range []*domain.Secret{active, expired} {
		if err := repo.Create(ctx, secret, time.Now()); err != nil {
			t.Fatalf("create %s: %v", secret.PublicID, err)
		}
	}

	activeHash := tokencrypto.TokenHash("active-session-token")
	if _, err := repo.StartRetrievalSession(
		ctx,
		"session-expiry-active",
		tokencrypto.TokenHash("blob-token-session-expiry-active"),
		activeHash,
		time.Now().Add(15*time.Minute),
		time.Now(),
	); err != nil {
		t.Fatalf("start active session: %v", err)
	}

	expiredHash := tokencrypto.TokenHash("expired-session-token")
	if _, err := repo.StartRetrievalSession(
		ctx,
		"session-expiry-expired",
		tokencrypto.TokenHash("blob-token-session-expiry-expired"),
		expiredHash,
		time.Now().Add(-time.Minute),
		time.Now(),
	); err != nil {
		t.Fatalf("start expired session: %v", err)
	}

	if _, err := repo.GetByRetrievalSession(ctx, "session-expiry-active", activeHash, time.Now()); err != nil {
		t.Fatalf("active session should validate: %v", err)
	}
	if _, err := repo.GetByRetrievalSession(ctx, "session-expiry-expired", expiredHash, time.Now()); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for expired session, got %v", err)
	}

	deleted, err := repo.DeleteExpiredRetrievalSessions(ctx, time.Now())
	if err != nil {
		t.Fatalf("delete expired sessions: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted sessions = %d, want 1", deleted)
	}

	if _, err := repo.GetByRetrievalSession(ctx, "session-expiry-active", activeHash, time.Now()); err != nil {
		t.Fatalf("active session should remain after cleanup: %v", err)
	}
}

func TestSecretRepo_StartRetrievalSession_BurnAfterRead(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("session-burn-001", time.Now().Add(1*time.Hour))
	secret.BurnAfterRead = true
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := repo.StartRetrievalSession(
		ctx,
		"session-burn-001",
		tokencrypto.TokenHash("blob-token-session-burn-001"),
		tokencrypto.TokenHash("session-token"),
		time.Now().Add(15*time.Minute),
		time.Now(),
	)
	if err != nil {
		t.Fatalf("start retrieval session: %v", err)
	}

	got, err := repo.GetByPublicID(ctx, "session-burn-001", time.Now())
	if err != nil {
		t.Fatalf("get after session: %v", err)
	}
	if got.RetrievedAt == nil {
		t.Fatal("retrieved_at should be set")
	}

	_, err = repo.StartRetrievalSession(
		ctx,
		"session-burn-001",
		tokencrypto.TokenHash("blob-token-session-burn-001"),
		tokencrypto.TokenHash("session-token-2"),
		time.Now().Add(15*time.Minute),
		time.Now(),
	)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second burn session, got %v", err)
	}
}

func TestSecretRepo_Delete(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("del-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(ctx, "del-001"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err := repo.Delete(ctx, "del-001")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestSecretRepo_DeleteExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	expired1 := newTestSecret("exp-del-001", time.Now().Add(-1*time.Hour))
	expired2 := newTestSecret("exp-del-002", time.Now().Add(-2*time.Hour))
	valid := newTestSecret("exp-del-003", time.Now().Add(1*time.Hour))

	for _, s := range []*domain.Secret{expired1, expired2, valid} {
		if err := repo.Create(ctx, s, time.Now()); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}

	noop := func(string) error { return nil }
	count, err := repo.DeleteExpired(ctx, time.Now(), testBatchSize, noop)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count.Removed != 2 {
		t.Errorf("deleted count = %d, want 2", count.Removed)
	}

	// Valid secret should still exist
	_, err = repo.GetByPublicID(ctx, "exp-del-003", time.Now())
	if err != nil {
		t.Fatalf("valid secret should still exist: %v", err)
	}
}

func TestSecretRepo_DeleteExpired_BurnAfterRead(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	// 1. Retrieved burn-after-read secret — should be deleted
	burnRetrieved := newTestSecret("burn-retr-001", time.Now().Add(1*time.Hour))
	burnRetrieved.BurnAfterRead = true
	if err := repo.Create(ctx, burnRetrieved, time.Now()); err != nil {
		t.Fatalf("create burn-retrieved: %v", err)
	}
	markRetrieved(t, pool, "burn-retr-001")

	// 2. Retrieved regular secret — should NOT be deleted
	regularRetrieved := newTestSecret("reg-retr-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, regularRetrieved, time.Now()); err != nil {
		t.Fatalf("create regular-retrieved: %v", err)
	}
	markRetrieved(t, pool, "reg-retr-001")

	// 3. Unretrieved burn-after-read secret — should NOT be deleted
	burnUnretrieved := newTestSecret("burn-unretr-001", time.Now().Add(1*time.Hour))
	burnUnretrieved.BurnAfterRead = true
	if err := repo.Create(ctx, burnUnretrieved, time.Now()); err != nil {
		t.Fatalf("create burn-unretrieved: %v", err)
	}

	noop := func(string) error { return nil }
	count, err := repo.DeleteExpired(ctx, time.Now(), testBatchSize, noop)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count.Removed != 1 {
		t.Errorf("deleted count = %d, want 1", count.Removed)
	}

	// Regular retrieved secret should still exist
	if _, err := repo.GetByPublicID(ctx, "reg-retr-001", time.Now()); err != nil {
		t.Fatalf("regular retrieved secret should still exist: %v", err)
	}

	// Unretrieved burn secret should still exist
	if _, err := repo.GetByPublicID(ctx, "burn-unretr-001", time.Now()); err != nil {
		t.Fatalf("unretrieved burn secret should still exist: %v", err)
	}
}

func TestSecretRepo_DeleteExpired_KeepsBurnedSecretWithActiveSession(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("burn-active-session", time.Now().Add(1*time.Hour))
	secret.BurnAfterRead = true
	if err := repo.Create(ctx, secret, time.Now()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.StartRetrievalSession(
		ctx,
		"burn-active-session",
		tokencrypto.TokenHash("blob-token-burn-active-session"),
		tokencrypto.TokenHash("session-active"),
		time.Now().Add(15*time.Minute),
		time.Now(),
	); err != nil {
		t.Fatalf("start retrieval session: %v", err)
	}

	noop := func(string) error { return nil }
	count, err := repo.DeleteExpired(ctx, time.Now(), testBatchSize, noop)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if count.Removed != 0 {
		t.Errorf("deleted count = %d, want 0", count.Removed)
	}

	if _, err := pool.Exec(ctx, "UPDATE retrieval_sessions SET expires_at = $2 WHERE public_id = $1", "burn-active-session", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("expire retrieval session: %v", err)
	}
	deletedSessions, err := repo.DeleteExpiredRetrievalSessions(ctx, time.Now())
	if err != nil {
		t.Fatalf("delete expired retrieval sessions: %v", err)
	}
	if deletedSessions != 1 {
		t.Errorf("deleted sessions = %d, want 1", deletedSessions)
	}

	count, err = repo.DeleteExpired(ctx, time.Now(), testBatchSize, noop)
	if err != nil {
		t.Fatalf("delete expired after session cleanup: %v", err)
	}
	if count.Removed != 1 {
		t.Errorf("deleted count after session cleanup = %d, want 1", count.Removed)
	}
}

func TestSecretRepo_DeleteExpired_Batches(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	now := time.Now()
	consumed := newTestSecret("batch-consumed", now.Add(time.Hour))
	consumed.BurnAfterRead = true
	for _, s := range []*domain.Secret{
		newTestSecret("batch-exp-3h", now.Add(-3*time.Hour)),
		newTestSecret("batch-exp-1h", now.Add(-1*time.Hour)),
		newTestSecret("batch-exp-2h", now.Add(-2*time.Hour)),
		consumed,
		newTestSecret("batch-live", now.Add(time.Hour)),
	} {
		if err := repo.Create(ctx, s, now); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}
	markRetrieved(t, pool, "batch-consumed")

	var seen []string
	record := func(storageKey string) error {
		seen = append(seen, storageKey)
		return nil
	}

	// Oldest first, never more than the limit per batch; expired and consumed
	// burn-after-read secrets come from the same backlog.
	for i, want := range []domain.CleanupBatch{{Found: 2, Removed: 2}, {Found: 2, Removed: 2}, {Found: 0, Removed: 0}} {
		got, err := repo.DeleteExpired(ctx, now, 2, record)
		if err != nil {
			t.Fatalf("batch %d: %v", i+1, err)
		}
		if got != want {
			t.Errorf("batch %d = %+v, want %+v", i+1, got, want)
		}
	}
	wantOrder := []string{"secrets/batch-exp-3h", "secrets/batch-exp-2h", "secrets/batch-exp-1h", "secrets/batch-consumed"}
	if strings.Join(seen, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("cleanup order = %v, want %v", seen, wantOrder)
	}
	if _, err := repo.GetByPublicID(ctx, "batch-live", now); err != nil {
		t.Errorf("live secret should survive: %v", err)
	}
}

func TestSecretRepo_DeleteExpired_SkipsRowsLockedElsewhere(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	for _, id := range []string{"locked-elsewhere", "free-to-delete"} {
		if err := repo.Create(ctx, newTestSecret(id, time.Now().Add(-time.Hour)), time.Now()); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	// Another replica's cleanup (or a request) holds one row.
	other, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = other.Rollback(ctx) }()
	if _, err := other.Exec(ctx, "SELECT 1 FROM secrets WHERE public_id = 'locked-elsewhere' FOR UPDATE"); err != nil {
		t.Fatalf("lock row: %v", err)
	}

	var seen []string
	got, err := repo.DeleteExpired(ctx, time.Now(), testBatchSize, func(storageKey string) error {
		seen = append(seen, storageKey)
		return nil
	})
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if got != (domain.CleanupBatch{Found: 1, Removed: 1}) || len(seen) != 1 || seen[0] != "secrets/free-to-delete" {
		t.Errorf("batch = %+v, saw %v; want only the unlocked row", got, seen)
	}
}

func TestSecretRepo_DeleteExpired_HookError(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	expired1 := newTestSecret("hook-err-001", time.Now().Add(-1*time.Hour))
	expired2 := newTestSecret("hook-err-002", time.Now().Add(-2*time.Hour))

	for _, s := range []*domain.Secret{expired1, expired2} {
		if err := repo.Create(ctx, s, time.Now()); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}

	// Hook fails for hook-err-001, succeeds for hook-err-002
	failOne := func(storageKey string) error {
		if storageKey == "secrets/hook-err-001" {
			return errors.New("S3 delete failed")
		}
		return nil
	}

	count, err := repo.DeleteExpired(ctx, time.Now(), testBatchSize, failOne)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count.Removed != 1 {
		t.Errorf("deleted count = %d, want 1", count.Removed)
	}

	// hook-err-001 should still exist (hook failed, row kept)
	// We can't use GetByPublicID because it filters by expires_at.
	// Instead, call DeleteExpired again with a noop — if it finds a row, it was kept.
	noop := func(string) error { return nil }
	count2, err := repo.DeleteExpired(ctx, time.Now(), testBatchSize, noop)
	if err != nil {
		t.Fatalf("second delete expired: %v", err)
	}
	if count2.Removed != 1 {
		t.Errorf("second pass count = %d, want 1 (the skipped row)", count2.Removed)
	}
}
