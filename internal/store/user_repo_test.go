package store_test

import (
	"context"
	"testing"

	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

func newTestUser(email string) *model.User {
	return &model.User{
		Email:        email,
		PasswordHash: "hashed-password-" + email,
		DisplayName:  "Test User",
	}
}

func TestUserRepo_CreateAndGetByEmail(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresUserRepo(pool)
	ctx := context.Background()

	user := newTestUser("alice@example.com")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if user.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
	if user.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at after create")
	}
	if user.UpdatedAt.IsZero() {
		t.Error("expected non-zero updated_at after create")
	}

	got, err := repo.GetByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}

	if got.ID != user.ID {
		t.Errorf("id = %d, want %d", got.ID, user.ID)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", got.Email, "alice@example.com")
	}
	if got.DisplayName != "Test User" {
		t.Errorf("display_name = %q, want %q", got.DisplayName, "Test User")
	}
	if got.PasswordHash != "hashed-password-alice@example.com" {
		t.Errorf("password_hash mismatch")
	}
}

func TestUserRepo_CreateDuplicateEmail(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresUserRepo(pool)
	ctx := context.Background()

	user1 := newTestUser("dup@example.com")
	if err := repo.Create(ctx, user1); err != nil {
		t.Fatalf("create first user: %v", err)
	}

	user2 := newTestUser("dup@example.com")
	err := repo.Create(ctx, user2)
	if err != store.ErrDuplicateEmail {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestUserRepo_GetByID(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresUserRepo(pool)
	ctx := context.Background()

	user := newTestUser("bob@example.com")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	got, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}

	if got.Email != "bob@example.com" {
		t.Errorf("email = %q, want %q", got.Email, "bob@example.com")
	}
}

func TestUserRepo_GetNotFound(t *testing.T) {
	pool := setupTestDB(t)
	repo := store.NewPostgresUserRepo(pool)
	ctx := context.Background()

	_, err := repo.GetByEmail(ctx, "nobody@example.com")
	if err != store.ErrNotFound {
		t.Fatalf("get by email: expected ErrNotFound, got %v", err)
	}

	_, err = repo.GetByID(ctx, 99999)
	if err != store.ErrNotFound {
		t.Fatalf("get by id: expected ErrNotFound, got %v", err)
	}
}
