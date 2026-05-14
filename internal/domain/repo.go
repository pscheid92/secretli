package domain

import (
	"context"
	"time"
)

type SecretRepo interface {
	Create(ctx context.Context, secret *Secret) error
	CreateUpload(ctx context.Context, secret *Secret) error
	GetByPublicID(ctx context.Context, publicID string) (*Secret, error)
	GetPendingUploadByPublicID(ctx context.Context, publicID string) (*Secret, error)
	ClaimBurnAfterRead(ctx context.Context, publicID, blobTokenHash string) error
	StartRetrievalSession(ctx context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt time.Time) (*Secret, error)
	GetByRetrievalSession(ctx context.Context, publicID, sessionTokenHash string) (*Secret, error)
	GetObject(ctx context.Context, publicID, objectKind string, objectIndex int32) (*SecretObject, error)
	CreateObject(ctx context.Context, object *SecretObject) error
	ListObjects(ctx context.Context, publicID string) ([]SecretObject, error)
	CompleteUpload(ctx context.Context, publicID string, blobSize int64, expiresAt time.Time) error
	Delete(ctx context.Context, publicID string) error
	DeleteExpired(ctx context.Context, beforeDelete func(publicID string, objects []SecretObject) error) (int64, error)
	DeleteExpiredRetrievalSessions(ctx context.Context) (int64, error)
}
