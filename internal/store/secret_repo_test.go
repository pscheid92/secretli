package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

func newTextSecret(publicID string, expiresAt time.Time) *model.Secret {
	data := "encrypted-data"
	return &model.Secret{
		PublicID:           publicID,
		RetrievalTokenHash: "retrieval-hash-" + publicID,
		DeletionTokenHash:  "deletion-hash-" + publicID,
		EncryptedData:      &data,
		Nonce:              "nonce-" + publicID,
		SecretType:         "text",
		BurnAfterRead:      false,
		PasswordProtected:  false,
		ExpiresAt:          expiresAt,
	}
}

func TestSecretRepo_CreateAndGet(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	secret := newTextSecret("pub-001", time.Now().Add(1*time.Hour))
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
	if got.RetrievalTokenHash != "retrieval-hash-pub-001" {
		t.Errorf("retrieval_token_hash = %q, want %q", got.RetrievalTokenHash, "retrieval-hash-pub-001")
	}
	if got.SecretType != "text" {
		t.Errorf("secret_type = %q, want %q", got.SecretType, "text")
	}
	if got.EncryptedData == nil || *got.EncryptedData != "encrypted-data" {
		t.Errorf("encrypted_data = %v, want %q", got.EncryptedData, "encrypted-data")
	}
	if got.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestSecretRepo_CreateDuplicate(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	secret := newTextSecret("dup-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create first: %v", err)
	}

	secret2 := newTextSecret("dup-001", time.Now().Add(2*time.Hour))
	err := repo.Create(ctx, secret2)
	if err != store.ErrDuplicate {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}

func TestSecretRepo_GetExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	secret := newTextSecret("expired-001", time.Now().Add(-1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create expired secret: %v", err)
	}

	_, err := repo.GetByPublicID(ctx, "expired-001")
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for expired secret, got %v", err)
	}
}

func TestSecretRepo_GetAndDelete(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	secret := newTextSecret("burn-001", time.Now().Add(1*time.Hour))
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
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after burn, got %v", err)
	}
}

func TestSecretRepo_SetRetrievedAt(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	secret := newTextSecret("retr-001", time.Now().Add(1*time.Hour))
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
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	secret := newTextSecret("del-001", time.Now().Add(1*time.Hour))
	if err := repo.Create(ctx, secret); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(ctx, "del-001"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	err := repo.Delete(ctx, "del-001")
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestSecretRepo_DeleteExpired(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresSecretRepo(pool)
	ctx := context.Background()

	// Create mix of expired and valid secrets
	storageKey := "files/expired-key"
	expired1 := newTextSecret("exp-del-001", time.Now().Add(-1*time.Hour))
	expired1.StorageKey = &storageKey

	expired2 := newTextSecret("exp-del-002", time.Now().Add(-2*time.Hour))
	// expired2 has no storage key

	valid := newTextSecret("exp-del-003", time.Now().Add(1*time.Hour))

	for _, s := range []*model.Secret{expired1, expired2, valid} {
		if err := repo.Create(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.PublicID, err)
		}
	}

	count, keys, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if count != 2 {
		t.Errorf("deleted count = %d, want 2", count)
	}

	if len(keys) != 1 || keys[0] != "files/expired-key" {
		t.Errorf("storage keys = %v, want [files/expired-key]", keys)
	}

	// Valid secret should still exist
	_, err = repo.GetByPublicID(ctx, "exp-del-003")
	if err != nil {
		t.Fatalf("valid secret should still exist: %v", err)
	}
}
