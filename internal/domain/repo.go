package domain

import (
	"context"
	"time"
)

type SecretRepo interface {
	Create(ctx context.Context, secret *Secret, now time.Time) error
	GetByPublicID(ctx context.Context, publicID string, now time.Time) (*Secret, error)
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
	// ClearUploadParts forgets every recorded part of a pending session so the
	// client can upload them again after the backend rejected them.
	ClearUploadParts(ctx context.Context, sessionID string) error
}

// Repo is the whole datastore, as wired at start-up. Consumers take the
// narrower interface they actually need.
type Repo interface {
	SecretRepo
	UploadSessionRepo
	UploadSessionCleanupRepo
}

type UploadSessionCleanupRepo interface {
	// AbortExpiredUploadSessions marks expired pending sessions aborted. The
	// callback runs before each row is updated and receives whether an active
	// secret row already exists for the session's public_id, so the caller can
	// remove an orphaned object left by a crash between storage completion and
	// the database commit.
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, beforeAbort func(session *UploadSession, secretExists bool) error) (int64, error)
}
