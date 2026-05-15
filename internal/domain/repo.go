package domain

import (
	"context"
	"time"
)

type SecretRepo interface {
	Create(ctx context.Context, secret *Secret, now time.Time) error
	GetByPublicID(ctx context.Context, publicID string, now time.Time) (*Secret, error)
	ClaimBurnAfterRead(ctx context.Context, publicID, blobTokenHash string, now time.Time) error
	StartRetrievalSession(ctx context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt, now time.Time) (*Secret, error)
	GetByRetrievalSession(ctx context.Context, publicID, sessionTokenHash string, now time.Time) (*Secret, error)
	Delete(ctx context.Context, publicID string) error
	DeleteExpired(ctx context.Context, now time.Time, beforeDelete func(publicID string) error) (int64, error)
	DeleteExpiredRetrievalSessions(ctx context.Context, now time.Time) (int64, error)
}

type UploadSessionRepo interface {
	CreateUploadSession(ctx context.Context, session *UploadSession) error
	GetUploadSession(ctx context.Context, sessionID string) (*UploadSession, []UploadPart, error)
	RecordUploadPart(ctx context.Context, part *UploadPart) (*UploadPart, error)
	CompleteUploadSession(ctx context.Context, sessionID string, secret *Secret, now time.Time) error
	AbortUploadSession(ctx context.Context, sessionID string, now time.Time) error
}

type UploadSessionCleanupRepo interface {
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, beforeAbort func(*UploadSession) error) (int64, error)
}
