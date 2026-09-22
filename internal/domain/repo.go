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
	DeleteExpired(ctx context.Context, now time.Time, beforeDelete func(storageKey string) error) (int64, error)
	DeleteExpiredRetrievalSessions(ctx context.Context, now time.Time) (int64, error)
}

type UploadSessionRepo interface {
	CreateUploadSession(ctx context.Context, session *UploadSession) error
	GetUploadSession(ctx context.Context, sessionID string) (*UploadSession, []UploadPart, error)
	RecordUploadPart(ctx context.Context, part *UploadPart) (*UploadPart, error)
	// CompleteUploadSession turns a pending session into a secret while holding
	// the session's row lock, so concurrent completes are serialized. finalize
	// runs under the lock with the recorded parts and must finish the storage
	// upload; its error is returned unchanged. Only if it succeeds is the
	// secret created and the session marked completed, in one transaction.
	//
	// A session that is already completed is returned without calling
	// finalize, which makes a repeated complete idempotent. An aborted session
	// yields ErrConflict.
	CompleteUploadSession(ctx context.Context, sessionID string, now time.Time, finalize func(session *UploadSession, parts []UploadPart) error) (*UploadSession, error)
	// AbortUploadSession moves a pending session to aborted. It returns
	// ErrConflict if the session is no longer pending, so a caller knows
	// whether it was this call that ended the session.
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
	// callback runs before each row is updated. A pending session never has a
	// secret pointing at its storage key (the secret is created in the same
	// transaction that completes the session), so the callback may delete the
	// session's object, e.g. one left by a crash between storage completion
	// and the database commit.
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, beforeAbort func(session *UploadSession) error) (int64, error)
	// DeleteFinishedUploadSessions purges completed and aborted session
	// tombstones that finished before the given time.
	DeleteFinishedUploadSessions(ctx context.Context, finishedBefore time.Time) (int64, error)
}
