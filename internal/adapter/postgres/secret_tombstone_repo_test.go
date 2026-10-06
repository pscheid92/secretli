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

// Postgres keeps microseconds, so moments that are compared are rounded first.
func moment(t time.Time) time.Time {
	return t.Truncate(time.Microsecond)
}

// openAs starts a retrieval session on a secret made by newTestSecret, as a
// recipient, or as the owner when asOwner is set.
func openAs(t *testing.T, repo *pgadapter.SecretRepo, publicID, session string, asOwner bool, now time.Time) (*domain.Secret, error) {
	t.Helper()
	deletionHash := ""
	if asOwner {
		deletionHash = tokencrypto.TokenHash("deletion-token-" + publicID)
	}
	return repo.StartRetrievalSession(
		context.Background(),
		publicID,
		tokencrypto.TokenHash("blob-token-"+publicID),
		deletionHash,
		tokencrypto.TokenHash(session),
		now.Add(15*time.Minute),
		now,
	)
}

func mustCreate(t *testing.T, repo *pgadapter.SecretRepo, secret *domain.Secret) {
	t.Helper()
	if err := repo.Create(context.Background(), secret, time.Now()); err != nil {
		t.Fatalf("create %s: %v", secret.PublicID, err)
	}
}

func mustOpen(t *testing.T, repo *pgadapter.SecretRepo, publicID, session string, asOwner bool, now time.Time) {
	t.Helper()
	if _, err := openAs(t, repo, publicID, session, asOwner, now); err != nil {
		t.Fatalf("open %s: %v", publicID, err)
	}
}

func mustTombstone(t *testing.T, repo *pgadapter.SecretRepo, publicID string) *domain.SecretTombstone {
	t.Helper()
	tomb, err := repo.GetTombstone(context.Background(), publicID, time.Now())
	if err != nil {
		t.Fatalf("tombstone of %s: %v", publicID, err)
	}
	return tomb
}

func sameMoment(got *time.Time, want time.Time) bool {
	return got != nil && got.Equal(want)
}

func TestSecretRepo_OpeningAOneTimeSecretLeavesATombstone(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	expiresAt := moment(time.Now().Add(time.Hour))
	secret := newTestSecret("tomb-opened", expiresAt)
	secret.BurnAfterRead = true
	mustCreate(t, repo, secret)
	now := moment(time.Now())

	mustOpen(t, repo, "tomb-opened", "session-opened", false, now)

	tomb := mustTombstone(t, repo, "tomb-opened")
	if tomb.Outcome != domain.TombstoneOpened || !tomb.BurnAfterRead || tomb.OpenedByOwner {
		t.Errorf("tombstone = %+v, want opened by a recipient", tomb)
	}
	if !tomb.EndedAt.Equal(now) || !sameMoment(tomb.FirstOpenedAt, now) {
		t.Errorf("ended_at = %v, first_opened_at = %v, want %v", tomb.EndedAt, tomb.FirstOpenedAt, now)
	}
	if want := expiresAt.Add(domain.TombstoneRetention); !tomb.KeepUntil.Equal(want) {
		t.Errorf("keep_until = %v, want a week past the expiry, %v", tomb.KeepUntil, want)
	}
	if tomb.MetadataTokenHash != secret.MetadataTokenHash || tomb.DeletionTokenHash != secret.DeletionTokenHash {
		t.Error("tombstone must carry the secret's token hashes")
	}
}

func TestSecretRepo_OwnerOpeningTheirOneTimeSecretIsRecordedAsSuch(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	secret := newTestSecret("tomb-owner", time.Now().Add(time.Hour))
	secret.BurnAfterRead = true
	mustCreate(t, repo, secret)

	mustOpen(t, repo, "tomb-owner", "session-owner", true, moment(time.Now()))

	tomb := mustTombstone(t, repo, "tomb-owner")
	if tomb.Outcome != domain.TombstoneOpened || !tomb.OpenedByOwner {
		t.Errorf("tombstone = %+v, want opened by the owner", tomb)
	}
	if tomb.FirstOpenedAt != nil {
		t.Errorf("first_opened_at = %v, want none: no recipient got it", tomb.FirstOpenedAt)
	}
}

func TestSecretRepo_ReusableSecretRemembersWhenARecipientFirstOpenedIt(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()
	mustCreate(t, repo, newTestSecret("reuse-first", time.Now().Add(time.Hour)))

	// The owner having a look is not a recipient.
	mustOpen(t, repo, "reuse-first", "session-owner-look", true, time.Now())
	got, err := repo.GetByPublicID(ctx, "reuse-first", time.Now())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RetrievedAt != nil {
		t.Fatalf("retrieved_at = %v after the owner's look, want none", got.RetrievedAt)
	}

	first := moment(time.Now())
	mustOpen(t, repo, "reuse-first", "session-first", false, first)
	mustOpen(t, repo, "reuse-first", "session-second", false, first.Add(time.Minute))

	got, err = repo.GetByPublicID(ctx, "reuse-first", time.Now())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !sameMoment(got.RetrievedAt, first) {
		t.Errorf("retrieved_at = %v, want the first recipient's %v", got.RetrievedAt, first)
	}
	if _, err := repo.GetTombstone(ctx, "reuse-first", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("a live secret has no tombstone, got %v", err)
	}
}

func TestSecretRepo_DeletingLeavesATombstone(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()
	expiresAt := moment(time.Now().Add(time.Hour))
	mustCreate(t, repo, newTestSecret("tomb-deleted", expiresAt))
	opened := moment(time.Now())
	mustOpen(t, repo, "tomb-deleted", "session-before-delete", false, opened)

	deleted := opened.Add(time.Minute)
	if err := repo.Delete(ctx, "tomb-deleted", deleted); err != nil {
		t.Fatalf("delete: %v", err)
	}

	tomb := mustTombstone(t, repo, "tomb-deleted")
	if tomb.Outcome != domain.TombstoneDeleted || tomb.BurnAfterRead || tomb.OpenedByOwner {
		t.Errorf("tombstone = %+v, want a deleted reusable secret", tomb)
	}
	if !tomb.EndedAt.Equal(deleted) || !sameMoment(tomb.FirstOpenedAt, opened) {
		t.Errorf("ended_at = %v, first_opened_at = %v, want %v and %v", tomb.EndedAt, tomb.FirstOpenedAt, deleted, opened)
	}
	if want := expiresAt.Add(domain.TombstoneRetention); !tomb.KeepUntil.Equal(want) {
		t.Errorf("keep_until = %v, want %v", tomb.KeepUntil, want)
	}
	if _, err := repo.GetByPublicID(ctx, "tomb-deleted", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("secret still there after delete: %v", err)
	}
}

func TestSecretRepo_AnOpenedOneTimeSecretStaysOpenedWhenDeleted(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	secret := newTestSecret("tomb-opened-then-deleted", time.Now().Add(time.Hour))
	secret.BurnAfterRead = true
	mustCreate(t, repo, secret)
	mustOpen(t, repo, "tomb-opened-then-deleted", "session-opened", false, time.Now())

	if err := repo.Delete(context.Background(), "tomb-opened-then-deleted", time.Now()); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if tomb := mustTombstone(t, repo, "tomb-opened-then-deleted"); tomb.Outcome != domain.TombstoneOpened {
		t.Errorf("outcome = %q, want the opening to stay on record", tomb.Outcome)
	}
}

func TestSecretRepo_CleanupLeavesTombstonesForExpiredSecrets(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()
	expiredAt := moment(time.Now().Add(-2 * time.Minute))
	mustCreate(t, repo, newTestSecret("expired-read", expiredAt))
	markRetrieved(t, pool, "expired-read")
	unread := newTestSecret("expired-unread", expiredAt)
	unread.BurnAfterRead = true
	mustCreate(t, repo, unread)
	consumed := newTestSecret("consumed", time.Now().Add(time.Hour))
	consumed.BurnAfterRead = true
	mustCreate(t, repo, consumed)
	mustOpen(t, repo, "consumed", "session-consumed", false, time.Now().Add(-20*time.Minute))
	if _, err := repo.DeleteExpiredRetrievalSessions(ctx, time.Now()); err != nil {
		t.Fatalf("end sessions: %v", err)
	}

	now := moment(time.Now())
	batch, err := repo.DeleteExpired(ctx, now, testBatchSize, func(string) error { return nil })
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if batch.Removed != 3 {
		t.Fatalf("removed = %d, want all three", batch.Removed)
	}

	read := mustTombstone(t, repo, "expired-read")
	if read.Outcome != domain.TombstoneExpired || read.FirstOpenedAt == nil || !read.EndedAt.Equal(expiredAt) {
		t.Errorf("read tombstone = %+v, want expired after being opened, ended at its expiry", read)
	}
	if want := now.Add(domain.TombstoneRetention); !read.KeepUntil.Equal(want) {
		t.Errorf("keep_until = %v, want a week past the cleanup, %v", read.KeepUntil, want)
	}
	if tomb := mustTombstone(t, repo, "expired-unread"); tomb.Outcome != domain.TombstoneExpired || tomb.FirstOpenedAt != nil {
		t.Errorf("unread tombstone = %+v, want expired unopened", tomb)
	}
	if tomb := mustTombstone(t, repo, "consumed"); tomb.Outcome != domain.TombstoneOpened {
		t.Errorf("consumed tombstone = %+v, want its opening kept", tomb)
	}
}

func TestSecretRepo_GetTombstoneReadsAnExpiredSecretTheCleanupHasNotReached(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()
	expiredAt := moment(time.Now().Add(-time.Minute))
	mustCreate(t, repo, newTestSecret("stale", expiredAt))
	markRetrieved(t, pool, "stale")
	mustCreate(t, repo, newTestSecret("live", time.Now().Add(time.Hour)))

	tomb := mustTombstone(t, repo, "stale")
	if tomb.Outcome != domain.TombstoneExpired || tomb.FirstOpenedAt == nil || !tomb.EndedAt.Equal(expiredAt) {
		t.Errorf("tombstone = %+v, want expired after being opened, ended at its expiry", tomb)
	}
	if _, err := repo.GetTombstone(ctx, "live", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("live secret: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetTombstone(ctx, "never-was", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown secret: err = %v, want ErrNotFound", err)
	}
}

func TestSecretRepo_DeleteExpiredTombstonesForgetsOldOnes(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()
	mustCreate(t, repo, newTestSecret("forgotten", time.Now().Add(time.Hour)))
	mustCreate(t, repo, newTestSecret("remembered", time.Now().Add(time.Hour)))
	for _, id := range []string{"forgotten", "remembered"} {
		if err := repo.Delete(ctx, id, time.Now()); err != nil {
			t.Fatalf("delete %s: %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE secret_tombstones SET keep_until = $2 WHERE public_id = $1", "forgotten", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("age tombstone: %v", err)
	}

	n, err := repo.DeleteExpiredTombstones(ctx, time.Now())
	if err != nil {
		t.Fatalf("delete expired tombstones: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	if _, err := repo.GetTombstone(ctx, "forgotten", time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("forgotten tombstone: err = %v, want ErrNotFound", err)
	}
	mustTombstone(t, repo, "remembered")
}
