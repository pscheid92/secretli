package store

import (
	"context"

	"github.com/pscheid92/secretli/internal/model"
)

type SessionRepo interface {
	Create(ctx context.Context, userID int64) (string, error)
	GetByIDWithUser(ctx context.Context, sessionID string) (*model.User, error)
	Delete(ctx context.Context, sessionID string) error
	DeleteExpiredSessions(ctx context.Context) (int64, error)
}
