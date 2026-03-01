package handler

import (
	"context"
	"testing"

	"github.com/pscheid92/secretli/internal/model"
)

func TestUserFromContext_WithUser(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := ContextWithUser(context.Background(), user)

	got := UserFromContext(ctx)
	if got == nil {
		t.Fatal("expected user from context, got nil")
	}
	if got.ID != 1 {
		t.Errorf("user ID = %d, want 1", got.ID)
	}
	if got.Email != "test@example.com" {
		t.Errorf("email = %q, want %q", got.Email, "test@example.com")
	}
}

func TestUserFromContext_WithoutUser(t *testing.T) {
	ctx := context.Background()
	got := UserFromContext(ctx)
	if got != nil {
		t.Errorf("expected nil user from empty context, got %v", got)
	}
}

func TestUserFromContext_WrongType(t *testing.T) {
	// Store a non-User value under the same key
	ctx := context.WithValue(context.Background(), userContextKey, "not a user")
	got := UserFromContext(ctx)
	if got != nil {
		t.Errorf("expected nil for wrong type, got %v", got)
	}
}
