package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/domain"
)

func newTestSecret(publicID string, expiresAt time.Time) *domain.Secret {
	return &domain.Secret{
		PublicID:      publicID,
		MetadataToken: "metadata-token-" + publicID,
		BlobToken:     "blob-token-" + publicID,
		DeletionToken: "deletion-token-" + publicID,
		EncryptedMeta: "v1$nonce$meta-" + publicID,
		BlobSize:      1024,
		BurnAfterRead: false,
		ExpiresAt:     expiresAt,
	}
}

func markRetrieved(t *testing.T, pool *pgxpool.Pool, publicID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE secrets SET retrieved_at = NOW() WHERE public_id = $1", publicID); err != nil {
		t.Fatalf("mark retrieved %s: %v", publicID, err)
	}
}

func TestSecretRepo_CreateAndGet(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("pub-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create secret: %v", err)
	}

	got, err := repo.GetByPublicID(ctx, "pub-001")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}

	if got.PublicID != "pub-001" {
		t.Errorf("public_id = %q, want %q", got.PublicID, "pub-001")
	}
	if got.MetadataToken != "metadata-token-pub-001" {
		t.Errorf("metadata_token = %q, want %q", got.MetadataToken, "metadata-token-pub-001")
	}
	if got.BlobToken != "blob-token-pub-001" {
		t.Errorf("blob_token = %q, want %q", got.BlobToken, "blob-token-pub-001")
	}
	if got.EncryptedMeta != "v1$nonce$meta-pub-001" {
		t.Errorf("encrypted_meta = %q, want %q", got.EncryptedMeta, "v1$nonce$meta-pub-001")
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
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create first: %v", err)
	}

	secret2 := newTestSecret("dup-001", time.Now().Add(2*time.Hour))
	err := repo.Create(ctx, secret2)
	if err != domain.ErrDuplicate {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestSecretRepo_GetExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("expired-001", time.Now().Add(-1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create expired secret: %v", err)
	}

	_, err := repo.GetByPublicID(ctx, "expired-001")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound for expired secret, got %v", err)
	}
}

func TestSecretRepo_ClaimBurnAfterRead(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("claim-001", time.Now().Add(1*time.Hour))
	secret.BurnAfterRead = true
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.ClaimBurnAfterRead(ctx, "claim-001", "blob-token-claim-001"); err != nil {
		t.Fatalf("claim burn-after-read: %v", err)
	}

	got, err := repo.GetByPublicID(ctx, "claim-001")
	if err != nil {
		t.Fatalf("get after claim: %v", err)
	}
	if got.RetrievedAt == nil {
		t.Fatal("expected retrieved_at to be set")
	}
	if got.RetrievedAt.IsZero() {
		t.Error("expected non-zero retrieved_at")
	}

	err = repo.ClaimBurnAfterRead(ctx, "claim-001", "blob-token-claim-001")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound on second claim, got %v", err)
	}
}

func TestSecretRepo_ClaimBurnAfterRead_InvalidInputs(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	burn := newTestSecret("claim-invalid-burn", time.Now().Add(1*time.Hour))
	burn.BurnAfterRead = true
	regular := newTestSecret("claim-invalid-regular", time.Now().Add(1*time.Hour))
	expired := newTestSecret("claim-invalid-expired", time.Now().Add(-1*time.Hour))
	expired.BurnAfterRead = true

	for _, s := range []*domain.Secret{burn, regular, expired} {
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}

	tests := []struct {
		name      string
		publicID  string
		blobToken string
	}{
		{name: "wrong token", publicID: "claim-invalid-burn", blobToken: "wrong-token"},
		{name: "regular secret", publicID: "claim-invalid-regular", blobToken: "blob-token-claim-invalid-regular"},
		{name: "expired secret", publicID: "claim-invalid-expired", blobToken: "blob-token-claim-invalid-expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.ClaimBurnAfterRead(ctx, tt.publicID, tt.blobToken)
			if err != domain.ErrNotFound {
				t.Fatalf("expected ErrNotFound, got %v", err)
			}
		})
	}
}

func TestSecretRepo_ClaimBurnAfterRead_ConcurrentOnlyOneSucceeds(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("claim-concurrent", time.Now().Add(1*time.Hour))
	secret.BurnAfterRead = true
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create: %v", err)
	}

	const claims = 12
	var wg sync.WaitGroup
	errs := make(chan error, claims)

	for range claims {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.ClaimBurnAfterRead(ctx, "claim-concurrent", "blob-token-claim-concurrent")
		}()
	}

	wg.Wait()
	close(errs)

	var success, notFound int
	for err := range errs {
		switch err {
		case nil:
			success++
		case domain.ErrNotFound:
			notFound++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if success != 1 {
		t.Errorf("successful claims = %d, want 1", success)
	}
	if notFound != claims-1 {
		t.Errorf("not found claims = %d, want %d", notFound, claims-1)
	}
}

func TestSecretRepo_Delete(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("del-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(ctx, "del-001"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err := repo.Delete(ctx, "del-001")
	if err != domain.ErrNotFound {
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
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}

	noop := func(string) error { return nil }
	count, err := repo.DeleteExpired(ctx, noop)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count != 2 {
		t.Errorf("deleted count = %d, want 2", count)
	}

	// Valid secret should still exist
	_, err = repo.GetByPublicID(ctx, "exp-del-003")
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
	if err := repo.Create(ctx, burnRetrieved); err != nil {
		t.Fatalf("create burn-retrieved: %v", err)
	}
	markRetrieved(t, pool, "burn-retr-001")

	// 2. Retrieved regular secret — should NOT be deleted
	regularRetrieved := newTestSecret("reg-retr-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, regularRetrieved); err != nil {
		t.Fatalf("create regular-retrieved: %v", err)
	}
	markRetrieved(t, pool, "reg-retr-001")

	// 3. Unretrieved burn-after-read secret — should NOT be deleted
	burnUnretrieved := newTestSecret("burn-unretr-001", time.Now().Add(1*time.Hour))
	burnUnretrieved.BurnAfterRead = true
	if err := repo.Create(ctx, burnUnretrieved); err != nil {
		t.Fatalf("create burn-unretrieved: %v", err)
	}

	noop := func(string) error { return nil }
	count, err := repo.DeleteExpired(ctx, noop)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count != 1 {
		t.Errorf("deleted count = %d, want 1", count)
	}

	// Regular retrieved secret should still exist
	if _, err := repo.GetByPublicID(ctx, "reg-retr-001"); err != nil {
		t.Fatalf("regular retrieved secret should still exist: %v", err)
	}

	// Unretrieved burn secret should still exist
	if _, err := repo.GetByPublicID(ctx, "burn-unretr-001"); err != nil {
		t.Fatalf("unretrieved burn secret should still exist: %v", err)
	}
}

func TestSecretRepo_DeleteExpired_HookError(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	expired1 := newTestSecret("hook-err-001", time.Now().Add(-1*time.Hour))
	expired2 := newTestSecret("hook-err-002", time.Now().Add(-2*time.Hour))

	for _, s := range []*domain.Secret{expired1, expired2} {
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}

	// Hook fails for hook-err-001, succeeds for hook-err-002
	failOne := func(id string) error {
		if id == "hook-err-001" {
			return errors.New("S3 delete failed")
		}
		return nil
	}

	count, err := repo.DeleteExpired(ctx, failOne)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count != 1 {
		t.Errorf("deleted count = %d, want 1", count)
	}

	// hook-err-001 should still exist (hook failed, row kept)
	// We can't use GetByPublicID because it filters by expires_at > NOW().
	// Instead, call DeleteExpired again with a noop — if it finds a row, it was kept.
	noop := func(string) error { return nil }
	count2, err := repo.DeleteExpired(ctx, noop)
	if err != nil {
		t.Fatalf("second delete expired: %v", err)
	}
	if count2 != 1 {
		t.Errorf("second pass count = %d, want 1 (the skipped row)", count2)
	}
}
