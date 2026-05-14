package domain

import (
	"context"
	"time"
)

type SecretRepo interface {
	Create(ctx context.Context, secret *Secret) error
	GetByPublicID(ctx context.Context, publicID string) (*Secret, error)
	ClaimBurnAfterRead(ctx context.Context, publicID, blobTokenHash string) error
	StartRetrievalSession(ctx context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt time.Time) (*Secret, error)
	GetByRetrievalSession(ctx context.Context, publicID, sessionTokenHash string) (*Secret, error)
	Delete(ctx context.Context, publicID string) error
	DeleteExpired(ctx context.Context, beforeDelete func(publicID string) error) (int64, error)
	DeleteExpiredRetrievalSessions(ctx context.Context) (int64, error)
}
