package postgres_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
)

func newTestUploadSession(sessionID, publicID string, uploadExpiresAt time.Time) *domain.UploadSession {
	now := time.Now()
	return &domain.UploadSession{
		SessionID:         sessionID,
		UploadTokenHash:   tokencrypto.TokenHash("upload-token-" + sessionID),
		PublicID:          publicID,
		StorageKey:        domain.UploadStorageKey(sessionID),
		S3UploadID:        "s3-upload-" + sessionID,
		BlobSize:          4096,
		MetadataTokenHash: tokencrypto.TokenHash("metadata-token-" + publicID),
		BlobTokenHash:     tokencrypto.TokenHash("blob-token-" + publicID),
		DeletionTokenHash: tokencrypto.TokenHash("deletion-token-" + publicID),
		EncryptedMeta:     "v2$nonce$meta-" + publicID,
		BurnAfterRead:     false,
		SecretExpiresAt:   now.Add(time.Hour),
		UploadExpiresAt:   uploadExpiresAt,
		State:             domain.UploadSessionStatePending,
		CreatedAt:         now,
	}
}

func newTestUploadPart(sessionID string, number int, offset, size int64) *domain.UploadPart {
	return &domain.UploadPart{
		SessionID:  sessionID,
		PartNumber: number,
		Offset:     offset,
		Size:       size,
		SHA256:     "sha-" + sessionID,
		ETag:       "etag-" + sessionID,
		CreatedAt:  time.Now(),
	}
}

func TestUploadSessionRepo_CreateAndGet(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	session := newTestUploadSession("us-create", "us-create-public", time.Now().Add(time.Hour))
	if err := repo.CreateUploadSession(ctx, session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	got, parts, err := repo.GetUploadSession(ctx, "us-create")
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if got.PublicID != session.PublicID || got.StorageKey != session.StorageKey || got.S3UploadID != session.S3UploadID || got.BlobSize != session.BlobSize {
		t.Errorf("session round trip mismatch: %+v", got)
	}
	if got.State != domain.UploadSessionStatePending {
		t.Errorf("state = %q, want pending", got.State)
	}
	if len(parts) != 0 {
		t.Errorf("parts = %d, want 0", len(parts))
	}

	if _, _, err := repo.GetUploadSession(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing session error = %v, want ErrNotFound", err)
	}
}

func TestUploadSessionRepo_CreateRejectsDuplicates(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	// An active secret already owns this public_id.
	if err := repo.Create(ctx, newTestSecret("us-dup-secret", time.Now().Add(time.Hour)), time.Now()); err != nil {
		t.Fatalf("create secret: %v", err)
	}
	err := repo.CreateUploadSession(ctx, newTestUploadSession("us-dup-1", "us-dup-secret", time.Now().Add(time.Hour)))
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("create over active secret error = %v, want ErrDuplicate", err)
	}

	// Only one pending session per public_id.
	if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-dup-2", "us-dup-pending", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("create first pending session: %v", err)
	}
	err = repo.CreateUploadSession(ctx, newTestUploadSession("us-dup-3", "us-dup-pending", time.Now().Add(time.Hour)))
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("create second pending session error = %v, want ErrDuplicate", err)
	}

	// After abort the public_id is free again.
	if err := repo.AbortUploadSession(ctx, "us-dup-2", time.Now()); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-dup-4", "us-dup-pending", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("create after abort: %v", err)
	}
}

func TestUploadSessionRepo_RecordUploadPart(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-parts", "us-parts-public", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	first, err := repo.RecordUploadPart(ctx, newTestUploadPart("us-parts", 1, 0, 2048))
	if err != nil {
		t.Fatalf("record part: %v", err)
	}
	if first.ETag != "etag-us-parts" {
		t.Errorf("etag = %q", first.ETag)
	}

	// Same content again is idempotent and returns the stored record.
	again, err := repo.RecordUploadPart(ctx, newTestUploadPart("us-parts", 1, 0, 2048))
	if err != nil {
		t.Fatalf("record same part again: %v", err)
	}
	if again.ETag != first.ETag || again.Offset != first.Offset {
		t.Errorf("idempotent record mismatch: %+v vs %+v", again, first)
	}

	// Different content for the same part number conflicts.
	conflict := newTestUploadPart("us-parts", 1, 0, 2048)
	conflict.SHA256 = "different"
	if _, err := repo.RecordUploadPart(ctx, conflict); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("conflicting part error = %v, want ErrConflict", err)
	}

	if _, err := repo.RecordUploadPart(ctx, newTestUploadPart("us-parts", 2, 2048, 2048)); err != nil {
		t.Fatalf("record second part: %v", err)
	}
	_, parts, err := repo.GetUploadSession(ctx, "us-parts")
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if len(parts) != 2 || parts[0].PartNumber != 1 || parts[1].PartNumber != 2 {
		t.Errorf("parts = %+v, want parts 1 and 2 in order", parts)
	}

	if err := repo.ClearUploadParts(ctx, "us-parts"); err != nil {
		t.Fatalf("clear parts: %v", err)
	}
	_, parts, err = repo.GetUploadSession(ctx, "us-parts")
	if err != nil {
		t.Fatalf("get upload session after clear: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("parts after clear = %d, want 0", len(parts))
	}
}

func TestUploadSessionRepo_Complete(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	session := newTestUploadSession("us-complete", "us-complete-public", time.Now().Add(time.Hour))
	if err := repo.CreateUploadSession(ctx, session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}
	if _, err := repo.RecordUploadPart(ctx, newTestUploadPart("us-complete", 1, 0, 4096)); err != nil {
		t.Fatalf("record part: %v", err)
	}

	var finalizedParts []domain.UploadPart
	completed, err := repo.CompleteUploadSession(ctx, "us-complete", time.Now(), func(locked *domain.UploadSession, parts []domain.UploadPart) error {
		if locked.EncryptedMeta != session.EncryptedMeta || locked.StorageKey != session.StorageKey {
			t.Errorf("finalize got session %+v, want the pending session's data", locked)
		}
		finalizedParts = parts
		return nil
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(finalizedParts) != 1 || finalizedParts[0].PartNumber != 1 {
		t.Errorf("finalize parts = %+v, want part 1", finalizedParts)
	}
	if completed.State != domain.UploadSessionStateCompleted || completed.SecretExpiresAt.Sub(session.SecretExpiresAt).Abs() > time.Millisecond {
		t.Errorf("completed session = %+v", completed)
	}

	secret, err := repo.GetByPublicID(ctx, "us-complete-public", time.Now())
	if err != nil {
		t.Fatalf("completed secret should be retrievable: %v", err)
	}
	if secret.StorageKey != session.StorageKey || secret.EncryptedMeta != session.EncryptedMeta || secret.BlobTokenHash != session.BlobTokenHash {
		t.Errorf("secret = %+v, want the session's share data and storage key", secret)
	}

	// Completing again is idempotent: the tombstone answers without running
	// finalize or creating anything.
	again, err := repo.CompleteUploadSession(ctx, "us-complete", time.Now(), func(*domain.UploadSession, []domain.UploadPart) error {
		t.Error("finalize must not run for a completed session")
		return nil
	})
	if err != nil {
		t.Fatalf("second complete: %v", err)
	}
	if again.State != domain.UploadSessionStateCompleted || !again.SecretExpiresAt.Equal(completed.SecretExpiresAt) {
		t.Errorf("second complete = %+v, want the completed tombstone", again)
	}

	if _, err := repo.CompleteUploadSession(ctx, "missing", time.Now(), noFinalize(t)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("complete missing error = %v, want ErrNotFound", err)
	}

	// Aborting a completed session reports that it is no longer pending.
	if err := repo.AbortUploadSession(ctx, "us-complete", time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("abort completed session error = %v, want ErrConflict", err)
	}
	if err := repo.AbortUploadSession(ctx, "missing", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("abort missing error = %v, want ErrNotFound", err)
	}
}

func TestUploadSessionRepo_CompleteScrubsShareMaterial(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	for _, s := range []*domain.UploadSession{
		newTestUploadSession("us-scrub-done", "us-scrub-done-public", time.Now().Add(time.Hour)),
		newTestUploadSession("us-scrub-aborted", "us-scrub-aborted-public", time.Now().Add(time.Hour)),
	} {
		if err := repo.CreateUploadSession(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.SessionID, err)
		}
		if _, err := repo.RecordUploadPart(ctx, newTestUploadPart(s.SessionID, 1, 0, 4096)); err != nil {
			t.Fatalf("record part: %v", err)
		}
	}
	if _, err := repo.CompleteUploadSession(ctx, "us-scrub-done", time.Now(), noopFinalize); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := repo.AbortUploadSession(ctx, "us-scrub-aborted", time.Now()); err != nil {
		t.Fatalf("abort: %v", err)
	}

	for _, id := range []string{"us-scrub-done", "us-scrub-aborted"} {
		var leftovers int
		if err := pool.QueryRow(ctx, `
			SELECT num_nonnulls(metadata_token_hash, blob_token_hash, deletion_token_hash, encrypted_meta)
			FROM upload_sessions WHERE session_id = $1`, id).Scan(&leftovers); err != nil {
			t.Fatalf("query %s: %v", id, err)
		}
		if leftovers != 0 {
			t.Errorf("%s keeps %d share-material columns, want 0", id, leftovers)
		}
	}

	var parts int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM upload_parts WHERE session_id = 'us-scrub-done'").Scan(&parts); err != nil {
		t.Fatalf("count parts: %v", err)
	}
	if parts != 0 {
		t.Errorf("completed session keeps %d parts, want 0", parts)
	}
}

func TestUploadSessionRepo_ConcurrentCompletesFinalizeOnce(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-race", "us-race-public", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	const callers = 5
	var finalizeCalls atomic.Int32
	errs := make([]error, callers)
	states := make([]string, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			session, err := repo.CompleteUploadSession(ctx, "us-race", time.Now(), func(*domain.UploadSession, []domain.UploadPart) error {
				finalizeCalls.Add(1)
				// Hold the lock long enough for the other callers to queue.
				time.Sleep(50 * time.Millisecond)
				return nil
			})
			errs[i] = err
			if session != nil {
				states[i] = session.State
			}
		})
	}
	wg.Wait()

	if got := finalizeCalls.Load(); got != 1 {
		t.Errorf("finalize ran %d times, want exactly 1", got)
	}
	for i := range callers {
		if errs[i] != nil || states[i] != domain.UploadSessionStateCompleted {
			t.Errorf("caller %d: state = %q, err = %v; want completed", i, states[i], errs[i])
		}
	}
	var secrets int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM secrets WHERE public_id = 'us-race-public'").Scan(&secrets); err != nil {
		t.Fatalf("count secrets: %v", err)
	}
	if secrets != 1 {
		t.Errorf("secrets = %d, want 1", secrets)
	}
}

func TestUploadSessionRepo_CompleteRollsBack(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	t.Run("finalize error", func(t *testing.T) {
		if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-fin-err", "us-fin-err-public", time.Now().Add(time.Hour))); err != nil {
			t.Fatalf("create upload session: %v", err)
		}
		wantErr := errors.New("storage rejected parts")
		_, err := repo.CompleteUploadSession(ctx, "us-fin-err", time.Now(), func(*domain.UploadSession, []domain.UploadPart) error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("complete error = %v, want finalize's error", err)
		}
		assertUploadSessionState(t, repo, "us-fin-err", domain.UploadSessionStatePending)
	})

	t.Run("secret already exists", func(t *testing.T) {
		if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-atomic", "us-atomic-public", time.Now().Add(time.Hour))); err != nil {
			t.Fatalf("create upload session: %v", err)
		}
		// A secret appears under the same public_id before completion.
		if err := repo.Create(ctx, newTestSecret("us-atomic-public", time.Now().Add(time.Hour)), time.Now()); err != nil {
			t.Fatalf("create secret: %v", err)
		}
		if _, err := repo.CompleteUploadSession(ctx, "us-atomic", time.Now(), noopFinalize); !errors.Is(err, domain.ErrDuplicate) {
			t.Fatalf("complete error = %v, want ErrDuplicate", err)
		}
		assertUploadSessionState(t, repo, "us-atomic", domain.UploadSessionStatePending)
	})

	t.Run("aborted session", func(t *testing.T) {
		if err := repo.CreateUploadSession(ctx, newTestUploadSession("us-was-aborted", "us-was-aborted-public", time.Now().Add(time.Hour))); err != nil {
			t.Fatalf("create upload session: %v", err)
		}
		if err := repo.AbortUploadSession(ctx, "us-was-aborted", time.Now()); err != nil {
			t.Fatalf("abort: %v", err)
		}
		if _, err := repo.CompleteUploadSession(ctx, "us-was-aborted", time.Now(), noFinalize(t)); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("complete aborted error = %v, want ErrConflict", err)
		}
	})
}

func TestUploadSessionRepo_AbortExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	expired := newTestUploadSession("us-exp", "us-exp-public", time.Now().Add(-time.Hour))
	live := newTestUploadSession("us-live", "us-live-public", time.Now().Add(time.Hour))
	for _, s := range []*domain.UploadSession{expired, live} {
		if err := repo.CreateUploadSession(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.SessionID, err)
		}
	}

	var seen []string
	count, err := repo.AbortExpiredUploadSessions(ctx, time.Now(), func(session *domain.UploadSession) error {
		seen = append(seen, session.SessionID)
		if session.StorageKey != expired.StorageKey {
			t.Errorf("storage key = %q, want %q", session.StorageKey, expired.StorageKey)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("abort expired: %v", err)
	}
	if count != 1 || len(seen) != 1 || seen[0] != "us-exp" {
		t.Errorf("aborted count = %d, callback saw %v; want only us-exp", count, seen)
	}
	assertUploadSessionState(t, repo, "us-exp", domain.UploadSessionStateAborted)
	assertUploadSessionState(t, repo, "us-live", domain.UploadSessionStatePending)

	// A failing callback keeps the row pending for the next cycle.
	retry := newTestUploadSession("us-exp-retry", "us-exp-retry-public", time.Now().Add(-time.Hour))
	if err := repo.CreateUploadSession(ctx, retry); err != nil {
		t.Fatalf("create retry: %v", err)
	}
	count, err = repo.AbortExpiredUploadSessions(ctx, time.Now(), func(*domain.UploadSession) error {
		return errors.New("storage unavailable")
	})
	if err != nil {
		t.Fatalf("abort expired with failing hook: %v", err)
	}
	if count != 0 {
		t.Errorf("aborted count with failing hook = %d, want 0", count)
	}
	assertUploadSessionState(t, repo, "us-exp-retry", domain.UploadSessionStatePending)
}

func TestUploadSessionRepo_DeleteFinished(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	longAgo := time.Now().Add(-2 * time.Hour)
	for _, id := range []string{"us-old-done", "us-old-aborted", "us-new-done", "us-pending"} {
		if err := repo.CreateUploadSession(ctx, newTestUploadSession(id, id+"-public", time.Now().Add(time.Hour))); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	if _, err := repo.CompleteUploadSession(ctx, "us-old-done", longAgo, noopFinalize); err != nil {
		t.Fatalf("complete old: %v", err)
	}
	if err := repo.AbortUploadSession(ctx, "us-old-aborted", longAgo); err != nil {
		t.Fatalf("abort old: %v", err)
	}
	if _, err := repo.CompleteUploadSession(ctx, "us-new-done", time.Now(), noopFinalize); err != nil {
		t.Fatalf("complete new: %v", err)
	}

	count, err := repo.DeleteFinishedUploadSessions(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("delete finished: %v", err)
	}
	if count != 2 {
		t.Errorf("deleted = %d, want 2", count)
	}
	for _, id := range []string{"us-old-done", "us-old-aborted"} {
		if _, _, err := repo.GetUploadSession(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: err = %v, want purged", id, err)
		}
	}
	assertUploadSessionState(t, repo, "us-new-done", domain.UploadSessionStateCompleted)
	assertUploadSessionState(t, repo, "us-pending", domain.UploadSessionStatePending)

	// Purging the tombstone does not touch the secret it produced.
	if _, err := repo.GetByPublicID(ctx, "us-old-done-public", time.Now()); err != nil {
		t.Errorf("secret from purged session: %v", err)
	}
}

func noopFinalize(*domain.UploadSession, []domain.UploadPart) error { return nil }

func noFinalize(t *testing.T) func(*domain.UploadSession, []domain.UploadPart) error {
	return func(*domain.UploadSession, []domain.UploadPart) error {
		t.Error("finalize must not run")
		return nil
	}
}

func assertUploadSessionState(t *testing.T, repo *pgadapter.SecretRepo, sessionID, want string) {
	t.Helper()
	got, _, err := repo.GetUploadSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get %s: %v", sessionID, err)
	}
	if got.State != want {
		t.Errorf("%s state = %q, want %q", sessionID, got.State, want)
	}
}
