package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

// createTestSecret inserts a secret and returns its internal DB ID.
func createTestSecret(t *testing.T, pool *pgxpool.Pool, publicID string) int64 {
	t.Helper()
	repo := store.NewPostgresSecretRepo(pool)
	secret := newTextSecret(publicID, time.Now().Add(1*time.Hour))
	if err := repo.Create(context.Background(), secret); err != nil {
		t.Fatalf("create test secret %s: %v", publicID, err)
	}

	// Fetch back to get the DB-generated ID
	got, err := repo.GetByPublicID(context.Background(), publicID)
	if err != nil {
		t.Fatalf("get test secret %s: %v", publicID, err)
	}
	return got.ID
}

func TestUserSecretRepo_LinkAndList(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userRepo := store.NewPostgresUserRepo(pool)
	user := &model.User{
		Email:        "linker@example.com",
		PasswordHash: "hash",
		DisplayName:  "Linker",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	secretID := createTestSecret(t, pool, "link-001")

	usRepo := store.NewPostgresUserSecretRepo(pool)
	if err := usRepo.LinkSecret(ctx, user.ID, secretID, "my-secret"); err != nil {
		t.Fatalf("link secret: %v", err)
	}

	summaries, total, err := usRepo.ListByUser(ctx, user.ID, 1, 10)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}

	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries len = %d, want 1", len(summaries))
	}
	if summaries[0].PublicID != "link-001" {
		t.Errorf("public_id = %q, want %q", summaries[0].PublicID, "link-001")
	}
	if summaries[0].Label != "my-secret" {
		t.Errorf("label = %q, want %q", summaries[0].Label, "my-secret")
	}
}

func TestUserSecretRepo_Pagination(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userRepo := store.NewPostgresUserRepo(pool)
	user := &model.User{
		Email:        "paginator@example.com",
		PasswordHash: "hash",
		DisplayName:  "Paginator",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	usRepo := store.NewPostgresUserSecretRepo(pool)

	// Create and link 5 secrets
	for i := 0; i < 5; i++ {
		publicID := fmt.Sprintf("page-%03d", i)
		secretID := createTestSecret(t, pool, publicID)
		if err := usRepo.LinkSecret(ctx, user.ID, secretID, publicID); err != nil {
			t.Fatalf("link secret %s: %v", publicID, err)
		}
	}

	// Page 1 with perPage=2
	summaries, total, err := usRepo.ListByUser(ctx, user.ID, 1, 2)
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(summaries) != 2 {
		t.Errorf("page 1 len = %d, want 2", len(summaries))
	}

	// Page 3 with perPage=2 should have 1 item
	summaries, total, err = usRepo.ListByUser(ctx, user.ID, 3, 2)
	if err != nil {
		t.Fatalf("list page 3: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(summaries) != 1 {
		t.Errorf("page 3 len = %d, want 1", len(summaries))
	}
}

func TestUserSecretRepo_EmptyList(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userRepo := store.NewPostgresUserRepo(pool)
	user := &model.User{
		Email:        "empty@example.com",
		PasswordHash: "hash",
		DisplayName:  "Empty",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	usRepo := store.NewPostgresUserSecretRepo(pool)
	summaries, total, err := usRepo.ListByUser(ctx, user.ID, 1, 10)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}

	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(summaries) != 0 {
		t.Errorf("summaries len = %d, want 0", len(summaries))
	}
}
