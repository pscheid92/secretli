package domain

import (
	"context"
	"time"
)

// CleanupBatch is the outcome of one cleanup batch.
type CleanupBatch struct {
	// Found is how many rows were due, at most the batch limit.
	Found int
	// Removed is how many of them were cleaned up. The rest failed their
	// storage callback and are left for a later cycle.
	Removed int
}

type SecretRepo interface {
	Create(ctx context.Context, secret *Secret, now time.Time) error
	GetByPublicID(ctx context.Context, publicID string, now time.Time) (*Secret, error)
	StartRetrievalSession(ctx context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt, now time.Time) (*Secret, error)
	GetByRetrievalSession(ctx context.Context, publicID, sessionTokenHash string, now time.Time) (*Secret, error)
	Delete(ctx context.Context, publicID string) error
	// DeleteExpired deletes one batch of at most limit secrets that are
	// expired, or burn-after-read and consumed with no retrieval session left,
	// oldest first. beforeDelete runs for each row while it is locked; rows it
	// fails for are kept. The batch commits on its own, so progress survives a
	// later failure.
	DeleteExpired(ctx context.Context, now time.Time, limit int, beforeDelete func(storageKey string) error) (CleanupBatch, error)
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

type TransferRepo interface {
	// CreateTransfer stores an open transfer, with the sender's offer, under
	// the smallest free nameplate up to maxNameplate, first closing transfers
	// that ran out. It returns ErrConflict when no nameplate is free and
	// ErrDuplicate when the transfer id is taken.
	CreateTransfer(ctx context.Context, t *Transfer, maxNameplate int, now time.Time) error
	// ClaimTransfer hands the active, unclaimed transfer under nameplate to a
	// receiver. It returns ErrConflict if that transfer was already claimed
	// and ErrNotFound if there is none.
	ClaimTransfer(ctx context.Context, nameplate int, receiverTokenHash string, now time.Time) (*Transfer, error)
	GetTransfer(ctx context.Context, transferID string) (*Transfer, error)
	// AnswerTransfer stores the receiver's answer if the transfer is active,
	// claimed and not answered yet, and reports whether it did.
	AnswerTransfer(ctx context.Context, transferID string, share, confirmation []byte, now time.Time) (bool, error)
	// DeliverTransfer stores the sealed link and closes the transfer as done
	// if it is active and answered, and reports whether it did.
	DeliverTransfer(ctx context.Context, transferID string, delivery []byte, now time.Time) (bool, error)
	// CloseTransfer ends an active transfer. It returns ErrNotFound if the
	// transfer does not exist or already ended.
	CloseTransfer(ctx context.Context, transferID, reason string, now time.Time) error
}

type TransferCleanupRepo interface {
	// DeleteEndedTransfers deletes transfers that expired or were closed
	// before the given time.
	DeleteEndedTransfers(ctx context.Context, endedBefore time.Time) (int64, error)
}

// Repo is the whole datastore, as wired at start-up. Consumers take the
// narrower interface they actually need.
type Repo interface {
	SecretRepo
	UploadSessionRepo
	UploadSessionCleanupRepo
	TransferRepo
	TransferCleanupRepo
}

type UploadSessionCleanupRepo interface {
	// AbortExpiredUploadSessions marks one batch of at most limit expired
	// pending sessions aborted, oldest first. The callback runs for each row
	// while it is locked; rows it fails for stay pending. A pending session
	// never has a secret pointing at its storage key (the secret is created in
	// the same transaction that completes the session), so the callback may
	// delete the session's object, e.g. one left by a crash between storage
	// completion and the database commit.
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, limit int, beforeAbort func(session *UploadSession) error) (CleanupBatch, error)
	// DeleteFinishedUploadSessions purges completed and aborted session
	// tombstones that finished before the given time.
	DeleteFinishedUploadSessions(ctx context.Context, finishedBefore time.Time) (int64, error)
}
