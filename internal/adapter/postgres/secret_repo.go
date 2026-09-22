package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pscheid92/secretli/internal/adapter/postgres/dbsqlc"
	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
)

type SecretRepo struct {
	q    *dbsqlc.Queries
	pool *pgxpool.Pool
}

func NewSecretRepo(pool *pgxpool.Pool) *SecretRepo {
	return &SecretRepo{q: dbsqlc.New(pool), pool: pool}
}

func (r *SecretRepo) Create(ctx context.Context, secret *domain.Secret, now time.Time) error {
	params := dbsqlc.CreateSecretParams{
		PublicID:          secret.PublicID,
		MetadataTokenHash: secret.MetadataTokenHash,
		BlobTokenHash:     secret.BlobTokenHash,
		DeletionTokenHash: secret.DeletionTokenHash,
		EncryptedMeta:     secret.EncryptedMeta,
		BlobSize:          secret.BlobSize,
		BurnAfterRead:     secret.BurnAfterRead,
		ExpiresAt:         timestamptz(secret.ExpiresAt),
		CreatedAt:         timestamptz(now),
		StorageKey:        secret.StorageKey,
	}
	err := r.q.CreateSecret(ctx, params)
	if err != nil && isDuplicateKeyError(err) {
		return domain.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert secret: %w", err)
	}
	return nil
}

func (r *SecretRepo) GetByPublicID(ctx context.Context, publicID string, now time.Time) (*domain.Secret, error) {
	row, err := r.q.GetSecretByPublicID(ctx, dbsqlc.GetSecretByPublicIDParams{
		PublicID: publicID,
		NowAt:    timestamptz(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret: %w", err)
	}
	return secretFromRow(row), nil
}

func (r *SecretRepo) StartRetrievalSession(ctx context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt, now time.Time) (*domain.Secret, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)

	secretRow, err := qtx.GetSecretByPublicIDForUpdate(ctx, dbsqlc.GetSecretByPublicIDForUpdateParams{
		PublicID: publicID,
		NowAt:    timestamptz(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret for retrieval session: %w", err)
	}
	secret := secretFromRow(secretRow)

	if secret.BurnAfterRead && secret.RetrievedAt != nil {
		return nil, domain.ErrNotFound
	}
	if !tokencrypto.TokensEqual(blobTokenHash, secret.BlobTokenHash) {
		return nil, domain.ErrForbidden
	}

	if secret.BurnAfterRead {
		n, err := qtx.ClaimBurnAfterRead(ctx, dbsqlc.ClaimBurnAfterReadParams{
			NowAt:         timestamptz(now),
			PublicID:      publicID,
			BlobTokenHash: blobTokenHash,
		})
		if err != nil {
			return nil, fmt.Errorf("claim burn-after-read for retrieval session: %w", err)
		}
		if n == 0 {
			return nil, domain.ErrNotFound
		}
	}

	if err := qtx.CreateRetrievalSession(ctx, dbsqlc.CreateRetrievalSessionParams{
		PublicID:         publicID,
		SessionTokenHash: sessionTokenHash,
		ExpiresAt:        timestamptz(expiresAt),
		CreatedAt:        timestamptz(now),
	}); err != nil {
		return nil, fmt.Errorf("insert retrieval session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit retrieval session: %w", err)
	}
	return secret, nil
}

func (r *SecretRepo) GetByRetrievalSession(ctx context.Context, publicID, sessionTokenHash string, now time.Time) (*domain.Secret, error) {
	secret, err := r.q.GetSecretByRetrievalSession(ctx, dbsqlc.GetSecretByRetrievalSessionParams{
		PublicID:         publicID,
		SessionTokenHash: sessionTokenHash,
		NowAt:            timestamptz(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("query retrieval session: %w", err)
	}
	return secretFromRow(secret), nil
}

func (r *SecretRepo) Delete(ctx context.Context, publicID string) error {
	n, err := r.q.DeleteSecret(ctx, publicID)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SecretRepo) DeleteExpired(ctx context.Context, now time.Time, beforeDelete func(storageKey string) error) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)

	expiredIDs, err := qtx.SelectExpiredSecretsForCleanup(ctx, timestamptz(now))
	if err != nil {
		return 0, fmt.Errorf("select expired: %w", err)
	}

	consumedIDs, err := qtx.SelectConsumedBurnAfterReadSecretsForCleanup(ctx, timestamptz(now))
	if err != nil {
		return 0, fmt.Errorf("select consumed burn-after-read: %w", err)
	}

	type doomed struct{ publicID, storageKey string }
	candidates := make([]doomed, 0, len(expiredIDs)+len(consumedIDs))
	for _, row := range expiredIDs {
		candidates = append(candidates, doomed{row.PublicID, row.StorageKey})
	}
	for _, row := range consumedIDs {
		candidates = append(candidates, doomed{row.PublicID, row.StorageKey})
	}

	var deleted int64
	for _, c := range candidates {
		if err := beforeDelete(c.storageKey); err != nil {
			slog.ErrorContext(ctx, "cleanup: beforeDelete failed, skipping", "public_id", c.publicID, "error", err)
			continue
		}
		if _, err := qtx.DeleteSecret(ctx, c.publicID); err != nil {
			slog.ErrorContext(ctx, "cleanup: delete row failed", "public_id", c.publicID, "error", err)
			continue
		}
		deleted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return deleted, nil
}

func (r *SecretRepo) DeleteExpiredRetrievalSessions(ctx context.Context, now time.Time) (int64, error) {
	n, err := r.q.DeleteExpiredRetrievalSessions(ctx, timestamptz(now))
	if err != nil {
		return 0, fmt.Errorf("delete expired retrieval sessions: %w", err)
	}
	return n, nil
}

func (r *SecretRepo) CreateUploadSession(ctx context.Context, session *domain.UploadSession) error {
	activeExists, err := r.q.SecretExistsByPublicID(ctx, session.PublicID)
	if err != nil {
		return fmt.Errorf("check active secret: %w", err)
	}
	if activeExists {
		return domain.ErrDuplicate
	}

	err = r.q.CreateUploadSession(ctx, dbsqlc.CreateUploadSessionParams{
		SessionID:         session.SessionID,
		PublicID:          session.PublicID,
		UploadTokenHash:   session.UploadTokenHash,
		MetadataTokenHash: text(session.MetadataTokenHash),
		BlobTokenHash:     text(session.BlobTokenHash),
		DeletionTokenHash: text(session.DeletionTokenHash),
		S3UploadID:        session.S3UploadID,
		BlobSize:          session.BlobSize,
		EncryptedMeta:     text(session.EncryptedMeta),
		BurnAfterRead:     session.BurnAfterRead,
		SecretExpiresAt:   timestamptz(session.SecretExpiresAt),
		UploadExpiresAt:   timestamptz(session.UploadExpiresAt),
		CreatedAt:         timestamptz(session.CreatedAt),
		StorageKey:        session.StorageKey,
	})
	if err != nil && isDuplicateKeyError(err) {
		return domain.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert upload session: %w", err)
	}
	return nil
}

func (r *SecretRepo) GetUploadSession(ctx context.Context, sessionID string) (*domain.UploadSession, []domain.UploadPart, error) {
	session, err := r.q.GetUploadSession(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("query upload session: %w", err)
	}

	parts, err := r.q.ListUploadPartsBySession(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("query upload parts: %w", err)
	}
	return uploadSessionFromRow(session), uploadPartsFromRows(parts), nil
}

func (r *SecretRepo) RecordUploadPart(ctx context.Context, part *domain.UploadPart) (*domain.UploadPart, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin upload part tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)

	existing, err := qtx.GetUploadPartForUpdate(ctx, dbsqlc.GetUploadPartForUpdateParams{
		SessionID:  part.SessionID,
		PartNumber: int32(part.PartNumber), //nolint:gosec // bounded by the handler
	})
	if err == nil {
		existingPart := uploadPartFromRow(existing)
		if existingPart.Offset != part.Offset || existingPart.Size != part.Size || existingPart.SHA256 != part.SHA256 {
			return nil, domain.ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit existing upload part: %w", err)
		}
		return existingPart, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("query upload part: %w", err)
	}

	inserted, err := qtx.CreateUploadPart(ctx, dbsqlc.CreateUploadPartParams{
		SessionID:  part.SessionID,
		PartNumber: int32(part.PartNumber), //nolint:gosec // bounded by the handler
		PartOffset: part.Offset,
		PartSize:   part.Size,
		PartSha256: part.SHA256,
		Etag:       part.ETag,
		CreatedAt:  timestamptz(part.CreatedAt),
	})
	if err != nil && isDuplicateKeyError(err) {
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("insert upload part: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit upload part: %w", err)
	}
	return uploadPartFromRow(inserted), nil
}

func (r *SecretRepo) CompleteUploadSession(ctx context.Context, sessionID string, now time.Time, finalize func(*domain.UploadSession, []domain.UploadPart) error) (*domain.UploadSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin complete upload session tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)

	row, err := qtx.GetUploadSessionForUpdate(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query upload session for complete: %w", err)
	}
	session := uploadSessionFromRow(row)
	switch session.State {
	case domain.UploadSessionStateCompleted:
		return session, nil
	case domain.UploadSessionStatePending:
	default:
		return nil, domain.ErrConflict
	}

	// Read the parts under the lock: the caller's earlier read may predate a
	// concurrent part upload or reset.
	partRows, err := qtx.ListUploadPartsBySession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query upload parts for complete: %w", err)
	}
	if err := finalize(session, uploadPartsFromRows(partRows)); err != nil {
		return nil, err
	}

	err = qtx.CreateSecret(ctx, dbsqlc.CreateSecretParams{
		PublicID:          session.PublicID,
		MetadataTokenHash: session.MetadataTokenHash,
		BlobTokenHash:     session.BlobTokenHash,
		DeletionTokenHash: session.DeletionTokenHash,
		EncryptedMeta:     session.EncryptedMeta,
		BlobSize:          session.BlobSize,
		BurnAfterRead:     session.BurnAfterRead,
		ExpiresAt:         timestamptz(session.SecretExpiresAt),
		CreatedAt:         timestamptz(now),
		StorageKey:        session.StorageKey,
	})
	if err != nil && isDuplicateKeyError(err) {
		return nil, domain.ErrDuplicate
	}
	if err != nil {
		return nil, fmt.Errorf("insert completed secret: %w", err)
	}

	if err := qtx.MarkUploadSessionCompleted(ctx, dbsqlc.MarkUploadSessionCompletedParams{
		NowAt:     timestamptz(now),
		SessionID: sessionID,
	}); err != nil {
		return nil, fmt.Errorf("mark upload session completed: %w", err)
	}
	if err := qtx.DeleteUploadPartsBySession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("delete completed upload parts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit complete upload session: %w", err)
	}

	session.State = domain.UploadSessionStateCompleted
	session.CompletedAt = &now
	session.MetadataTokenHash = ""
	session.BlobTokenHash = ""
	session.DeletionTokenHash = ""
	session.EncryptedMeta = ""
	return session, nil
}

func (r *SecretRepo) AbortUploadSession(ctx context.Context, sessionID string, now time.Time) error {
	n, err := r.q.MarkUploadSessionAborted(ctx, dbsqlc.MarkUploadSessionAbortedParams{
		NowAt:     timestamptz(now),
		SessionID: sessionID,
	})
	if err != nil {
		return fmt.Errorf("abort upload session: %w", err)
	}
	if n == 0 {
		exists, err := r.q.UploadSessionExists(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("check upload session exists: %w", err)
		}
		if !exists {
			return domain.ErrNotFound
		}
		return domain.ErrConflict
	}
	return nil
}

func (r *SecretRepo) ClearUploadParts(ctx context.Context, sessionID string) error {
	if err := r.q.DeleteUploadPartsBySession(ctx, sessionID); err != nil {
		return fmt.Errorf("clear upload parts: %w", err)
	}
	return nil
}

func (r *SecretRepo) AbortExpiredUploadSessions(ctx context.Context, now time.Time, beforeAbort func(session *domain.UploadSession) error) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin expired upload session tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)

	rows, err := qtx.ListExpiredUploadSessionsForUpdate(ctx, timestamptz(now))
	if err != nil {
		return 0, fmt.Errorf("select expired upload sessions: %w", err)
	}

	var aborted int64
	for _, row := range rows {
		session := uploadSessionFromRow(row)
		if err := beforeAbort(session); err != nil {
			slog.ErrorContext(ctx, "cleanup: abort multipart upload failed, skipping", "session_id", session.SessionID, "error", err)
			continue
		}
		if _, err := qtx.MarkUploadSessionAborted(ctx, dbsqlc.MarkUploadSessionAbortedParams{
			NowAt:     timestamptz(now),
			SessionID: session.SessionID,
		}); err != nil {
			slog.ErrorContext(ctx, "cleanup: mark upload session aborted failed", "session_id", session.SessionID, "error", err)
			continue
		}
		aborted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit expired upload sessions: %w", err)
	}
	return aborted, nil
}

func (r *SecretRepo) DeleteFinishedUploadSessions(ctx context.Context, finishedBefore time.Time) (int64, error) {
	n, err := r.q.DeleteFinishedUploadSessions(ctx, timestamptz(finishedBefore))
	if err != nil {
		return 0, fmt.Errorf("delete finished upload sessions: %w", err)
	}
	return n, nil
}

func secretFromRow(row dbsqlc.Secret) *domain.Secret {
	return &domain.Secret{
		PublicID:          row.PublicID,
		MetadataTokenHash: row.MetadataTokenHash,
		BlobTokenHash:     row.BlobTokenHash,
		DeletionTokenHash: row.DeletionTokenHash,
		EncryptedMeta:     row.EncryptedMeta,
		BlobSize:          row.BlobSize,
		BurnAfterRead:     row.BurnAfterRead,
		ExpiresAt:         row.ExpiresAt.Time,
		CreatedAt:         row.CreatedAt.Time,
		RetrievedAt:       pointerFromTimestamp(row.RetrievedAt),
		StorageKey:        row.StorageKey,
	}
}

func uploadSessionFromRow(row dbsqlc.UploadSession) *domain.UploadSession {
	return &domain.UploadSession{
		SessionID:         row.SessionID,
		UploadTokenHash:   row.UploadTokenHash,
		PublicID:          row.PublicID,
		StorageKey:        row.StorageKey,
		S3UploadID:        row.S3UploadID,
		BlobSize:          row.BlobSize,
		MetadataTokenHash: row.MetadataTokenHash.String,
		BlobTokenHash:     row.BlobTokenHash.String,
		DeletionTokenHash: row.DeletionTokenHash.String,
		EncryptedMeta:     row.EncryptedMeta.String,
		BurnAfterRead:     row.BurnAfterRead,
		SecretExpiresAt:   row.SecretExpiresAt.Time,
		UploadExpiresAt:   row.UploadExpiresAt.Time,
		State:             row.State,
		CreatedAt:         row.CreatedAt.Time,
		CompletedAt:       pointerFromTimestamp(row.CompletedAt),
		AbortedAt:         pointerFromTimestamp(row.AbortedAt),
	}
}

func uploadPartsFromRows(rows []dbsqlc.UploadPart) []domain.UploadPart {
	parts := make([]domain.UploadPart, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, *uploadPartFromRow(row))
	}
	return parts
}

func uploadPartFromRow(row dbsqlc.UploadPart) *domain.UploadPart {
	return &domain.UploadPart{
		SessionID:  row.SessionID,
		PartNumber: int(row.PartNumber),
		Offset:     row.PartOffset,
		Size:       row.PartSize,
		SHA256:     row.PartSha256,
		ETag:       row.Etag,
		CreatedAt:  row.CreatedAt.Time,
	}
}

func pointerFromTimestamp(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func text(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func isDuplicateKeyError(err error) bool {
	if err, ok := errors.AsType[*pgconn.PgError](err); ok {
		return err.Code == "23505"
	}
	return false
}
