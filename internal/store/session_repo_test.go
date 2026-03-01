package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
)

// createTestUser is a helper that inserts a user and returns their ID.
func createTestUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	userRepo := store.NewPostgresUserRepo(pool)
	user := &model.User{
		Email:        email,
		PasswordHash: "hash-" + email,
		DisplayName:  "Test",
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return user.ID
}

func TestSessionRepo_CreateAndGet(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userID := createTestUser(t, pool, "session-user@example.com")
	repo := store.NewPostgresSessionRepo(pool, 1*time.Hour)

	sessionID, err := repo.Create(ctx, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	user, err := repo.GetByIDWithUser(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session with user: %v", err)
	}

	if user.ID != userID {
		t.Errorf("user id = %d, want %d", user.ID, userID)
	}
	if user.Email != "session-user@example.com" {
		t.Errorf("email = %q, want %q", user.Email, "session-user@example.com")
	}
}

func TestSessionRepo_GetExpired(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userID := createTestUser(t, pool, "expired-session@example.com")
	repo := store.NewPostgresSessionRepo(pool, 1*time.Hour)

	sessionID, err := repo.Create(ctx, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Manually expire the session via SQL
	_, err = pool.Exec(ctx, `UPDATE sessions SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, sessionID)
	if err != nil {
		t.Fatalf("expire session: %v", err)
	}

	_, err = repo.GetByIDWithUser(ctx, sessionID)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound for expired session, got %v", err)
	}
}

func TestSessionRepo_Delete(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userID := createTestUser(t, pool, "delete-session@example.com")
	repo := store.NewPostgresSessionRepo(pool, 1*time.Hour)

	sessionID, err := repo.Create(ctx, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := repo.Delete(ctx, sessionID); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	_, err = repo.GetByIDWithUser(ctx, sessionID)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSessionRepo_DeleteExpiredSessions(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	userID := createTestUser(t, pool, "cleanup-session@example.com")
	repo := store.NewPostgresSessionRepo(pool, 1*time.Hour)

	// Create 3 sessions
	for i := 0; i < 3; i++ {
		if _, err := repo.Create(ctx, userID); err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}

	// Expire 2 of them
	_, err := pool.Exec(ctx, `
		UPDATE sessions SET expires_at = NOW() - INTERVAL '1 hour'
		WHERE id IN (SELECT id FROM sessions ORDER BY created_at LIMIT 2)`)
	if err != nil {
		t.Fatalf("expire sessions: %v", err)
	}

	count, err := repo.DeleteExpiredSessions(ctx)
	if err != nil {
		t.Fatalf("delete expired sessions: %v", err)
	}

	if count != 2 {
		t.Errorf("deleted count = %d, want 2", count)
	}
}
