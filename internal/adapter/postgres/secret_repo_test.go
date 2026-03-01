package postgres_test

import (
	"context"
	"testing"
	"time"

	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/domain"
)

func newTestSecret(publicID string, expiresAt time.Time) *domain.Secret {
	return &domain.Secret{
		PublicID:          publicID,
		RetrievalToken:    "retrieval-token-" + publicID,
		DeletionToken:     "deletion-token-" + publicID,
		EncryptedMeta:     "v1$nonce$meta-" + publicID,
		BlobSize:          1024,
		BurnAfterRead: false,
		ExpiresAt:     expiresAt,
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
	if got.RetrievalToken != "retrieval-token-pub-001" {
		t.Errorf("retrieval_token = %q, want %q", got.RetrievalToken, "retrieval-token-pub-001")
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

func TestSecretRepo_GetAndDelete(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("burn-001", time.Now().Add(1*time.Hour))
	secret.BurnAfterRead = true
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetAndDeleteByPublicID(ctx, "burn-001")
	if err != nil {
		t.Fatalf("get and delete: %v", err)
	}
	if got.PublicID != "burn-001" {
		t.Errorf("public_id = %q, want %q", got.PublicID, "burn-001")
	}
	if !got.BurnAfterRead {
		t.Error("expected burn_after_read to be true")
	}

	// Should be gone now
	_, err = repo.GetByPublicID(ctx, "burn-001")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound after burn, got %v", err)
	}
}

func TestSecretRepo_SetRetrievedAt(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	ctx := context.Background()

	secret := newTestSecret("retr-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.SetRetrievedAt(ctx, "retr-001"); err != nil {
		t.Fatalf("set retrieved_at: %v", err)
	}

	got, err := repo.GetByPublicID(ctx, "retr-001")
	if err != nil {
		t.Fatalf("get after set retrieved_at: %v", err)
	}
	if got.RetrievedAt == nil {
		t.Fatal("expected retrieved_at to be set")
	}
	if got.RetrievedAt.IsZero() {
		t.Error("expected non-zero retrieved_at")
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

	count, publicIDs, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count != 2 {
		t.Errorf("deleted count = %d, want 2", count)
	}

	if len(publicIDs) != 2 {
		t.Errorf("public IDs count = %d, want 2", len(publicIDs))
	}

	// Valid secret should still exist
	_, err = repo.GetByPublicID(ctx, "exp-del-003")
	if err != nil {
		t.Fatalf("valid secret should still exist: %v", err)
	}
}
