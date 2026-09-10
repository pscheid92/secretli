package postgres_test

import (
	"context"
	"errors"
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
	if got.PublicID != session.PublicID || got.S3UploadID != session.S3UploadID || got.BlobSize != session.BlobSize {
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

	secret := newTestSecret("us-complete-public", session.SecretExpiresAt)
	if err := repo.CompleteUploadSession(ctx, "us-complete", secret, time.Now()); err != nil {
		t.Fatalf("complete: %v", err)
	}

	got, _, err := repo.GetUploadSession(ctx, "us-complete")
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if got.State != domain.UploadSessionStateCompleted || got.CompletedAt == nil {
		t.Errorf("session state = %q, completed_at = %v; want completed", got.State, got.CompletedAt)
	}
	if _, err := repo.GetByPublicID(ctx, "us-complete-public", time.Now()); err != nil {
		t.Fatalf("completed secret should be retrievable: %v", err)
	}

	// Completing twice is a conflict and does not duplicate the secret.
	if err := repo.CompleteUploadSession(ctx, "us-complete", secret, time.Now()); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second complete error = %v, want ErrConflict", err)
	}
	if err := repo.CompleteUploadSession(ctx, "missing", secret, time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("complete missing error = %v, want ErrNotFound", err)
	}

	// Aborting a completed session is a no-op rather than an error.
	if err := repo.AbortUploadSession(ctx, "us-complete", time.Now()); err != nil {
		t.Fatalf("abort completed session: %v", err)
	}
	if err := repo.AbortUploadSession(ctx, "missing", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("abort missing error = %v, want ErrNotFound", err)
	}
}

func TestUploadSessionRepo_CompleteIsAtomicWhenSecretExists(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	session := newTestUploadSession("us-atomic", "us-atomic-public", time.Now().Add(time.Hour))
	if err := repo.CreateUploadSession(ctx, session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}
	// A secret appears under the same public_id before completion.
	if err := repo.Create(ctx, newTestSecret("us-atomic-public", time.Now().Add(time.Hour)), time.Now()); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	err := repo.CompleteUploadSession(ctx, "us-atomic", newTestSecret("us-atomic-public", time.Now().Add(time.Hour)), time.Now())
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("complete error = %v, want ErrDuplicate", err)
	}
	got, _, err := repo.GetUploadSession(ctx, "us-atomic")
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if got.State != domain.UploadSessionStatePending {
		t.Errorf("session state = %q, want pending (transaction rolled back)", got.State)
	}
}

func TestUploadSessionRepo_AbortExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	expiredOrphan := newTestUploadSession("us-exp-orphan", "us-exp-orphan-public", time.Now().Add(-time.Hour))
	expiredWithSecret := newTestUploadSession("us-exp-secret", "us-exp-secret-public", time.Now().Add(-time.Hour))
	live := newTestUploadSession("us-live", "us-live-public", time.Now().Add(time.Hour))
	for _, s := range []*domain.UploadSession{expiredOrphan, expiredWithSecret, live} {
		if err := repo.CreateUploadSession(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.SessionID, err)
		}
	}
	// Simulate a crash after storage completion and DB commit of the secret
	// but before the session row was marked, so a secret row exists.
	if _, err := pool.Exec(ctx, "INSERT INTO secrets (public_id, metadata_token_hash, blob_token_hash, deletion_token_hash, encrypted_meta, blob_size, burn_after_read, expires_at, created_at) VALUES ($1, 'm', 'b', 'd', 'meta', 1, false, $2, $3)", "us-exp-secret-public", time.Now().Add(time.Hour), time.Now()); err != nil {
		t.Fatalf("insert secret: %v", err)
	}

	seen := map[string]bool{}
	count, err := repo.AbortExpiredUploadSessions(ctx, time.Now(), func(session *domain.UploadSession, secretExists bool) error {
		seen[session.SessionID] = secretExists
		if session.SessionID == "us-exp-orphan" && secretExists {
			t.Error("orphan session reported as having a secret")
		}
		if session.SessionID == "us-exp-secret" && !secretExists {
			t.Error("session with secret reported as orphan")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("abort expired: %v", err)
	}
	if count != 2 {
		t.Errorf("aborted count = %d, want 2", count)
	}
	if len(seen) != 2 || seen["us-live"] {
		t.Errorf("callback saw %v, want only the two expired sessions", seen)
	}

	for id, want := range map[string]string{
		"us-exp-orphan": domain.UploadSessionStateAborted,
		"us-exp-secret": domain.UploadSessionStateAborted,
		"us-live":       domain.UploadSessionStatePending,
	} {
		got, _, err := repo.GetUploadSession(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got.State != want {
			t.Errorf("%s state = %q, want %q", id, got.State, want)
		}
	}

	// A failing callback keeps the row pending for the next cycle.
	retry := newTestUploadSession("us-exp-retry", "us-exp-retry-public", time.Now().Add(-time.Hour))
	if err := repo.CreateUploadSession(ctx, retry); err != nil {
		t.Fatalf("create retry: %v", err)
	}
	count, err = repo.AbortExpiredUploadSessions(ctx, time.Now(), func(*domain.UploadSession, bool) error {
		return errors.New("storage unavailable")
	})
	if err != nil {
		t.Fatalf("abort expired with failing hook: %v", err)
	}
	if count != 0 {
		t.Errorf("aborted count with failing hook = %d, want 0", count)
	}
	got, _, err := repo.GetUploadSession(ctx, "us-exp-retry")
	if err != nil {
		t.Fatalf("get retry: %v", err)
	}
	if got.State != domain.UploadSessionStatePending {
		t.Errorf("retry state = %q, want pending", got.State)
	}
}
