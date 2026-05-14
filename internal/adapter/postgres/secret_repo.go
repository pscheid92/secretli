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

func (r *SecretRepo) Create(ctx context.Context, secret *domain.Secret) error {
	params := dbsqlc.CreateSecretParams{
		PublicID:          secret.PublicID,
		MetadataTokenHash: secret.MetadataTokenHash,
		BlobTokenHash:     secret.BlobTokenHash,
		DeletionTokenHash: secret.DeletionTokenHash,
		EncryptedMeta:     secret.EncryptedMeta,
		BlobSize:          secret.BlobSize,
		BurnAfterRead:     secret.BurnAfterRead,
		ExpiresAt:         pgtype.Timestamptz{Time: secret.ExpiresAt, Valid: true},
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

func (r *SecretRepo) CreateUpload(ctx context.Context, secret *domain.Secret) error {
	params := dbsqlc.CreateChunkedUploadParams{
		PublicID:                  secret.PublicID,
		MetadataTokenHash:         secret.MetadataTokenHash,
		BlobTokenHash:             secret.BlobTokenHash,
		DeletionTokenHash:         secret.DeletionTokenHash,
		EncryptedMeta:             secret.EncryptedMeta,
		BlobSize:                  secret.BlobSize,
		BurnAfterRead:             secret.BurnAfterRead,
		ExpiresAt:                 pgtype.Timestamptz{Time: secret.ExpiresAt, Valid: true},
		ExpirationDurationSeconds: pgtype.Int8{Int64: secret.ExpirationDurationSeconds, Valid: true},
		UploadTokenHash:           pgtype.Text{String: secret.UploadTokenHash, Valid: true},
		ChunkSize:                 pgtype.Int8{Int64: secret.ChunkSize, Valid: true},
		ChunkCount:                pgtype.Int4{Int32: secret.ChunkCount, Valid: true},
		EncryptedTotalSize:        pgtype.Int8{Int64: secret.EncryptedTotalSize, Valid: true},
	}
	if secret.UploadExpiresAt != nil {
		params.UploadExpiresAt = pgtype.Timestamptz{Time: *secret.UploadExpiresAt, Valid: true}
	}

	err := r.q.CreateChunkedUpload(ctx, params)
	if err != nil && isDuplicateKeyError(err) {
		return domain.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert chunked upload: %w", err)
	}
	return nil
}

func (r *SecretRepo) GetByPublicID(ctx context.Context, publicID string) (*domain.Secret, error) {
	row, err := r.q.GetSecretByPublicID(ctx, publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret: %w", err)
	}
	return secretFromRow(row), nil
}

func (r *SecretRepo) GetPendingUploadByPublicID(ctx context.Context, publicID string) (*domain.Secret, error) {
	row, err := r.q.GetPendingUploadByPublicID(ctx, publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query pending upload: %w", err)
	}
	return secretFromPendingRow(row), nil
}

func (r *SecretRepo) ClaimBurnAfterRead(ctx context.Context, publicID, blobTokenHash string) error {
	n, err := r.q.ClaimBurnAfterRead(ctx, dbsqlc.ClaimBurnAfterReadParams{
		PublicID:      publicID,
		BlobTokenHash: blobTokenHash,
	})
	if err != nil {
		return fmt.Errorf("claim burn-after-read: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SecretRepo) StartRetrievalSession(ctx context.Context, publicID, blobTokenHash, sessionTokenHash string, expiresAt time.Time) (*domain.Secret, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	secret, err := scanSecretRow(tx.QueryRow(ctx, `
SELECT public_id, metadata_token_hash, blob_token_hash, deletion_token_hash,
    encrypted_meta, blob_size, burn_after_read, expires_at, created_at, retrieved_at,
    storage_version, status, expiration_duration_seconds, upload_token_hash,
    upload_expires_at, chunk_size, chunk_count, encrypted_total_size, completed_at
FROM secrets
WHERE public_id = $1 AND status = 'active' AND expires_at > NOW()
FOR UPDATE
`, publicID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret for retrieval session: %w", err)
	}

	if secret.BurnAfterRead && secret.RetrievedAt != nil {
		return nil, domain.ErrNotFound
	}
	if !tokencrypto.TokensEqual(blobTokenHash, secret.BlobTokenHash) {
		return nil, domain.ErrForbidden
	}

	if secret.BurnAfterRead {
		tag, err := tx.Exec(ctx, `
UPDATE secrets SET retrieved_at = NOW()
WHERE public_id = $1
  AND burn_after_read = true
  AND retrieved_at IS NULL
  AND status = 'active'
  AND expires_at > NOW()
`, publicID)
		if err != nil {
			return nil, fmt.Errorf("claim burn-after-read for retrieval session: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, domain.ErrNotFound
		}
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO retrieval_sessions (public_id, session_token_hash, expires_at)
VALUES ($1, $2, $3)
`, publicID, sessionTokenHash, pgtype.Timestamptz{Time: expiresAt, Valid: true}); err != nil {
		return nil, fmt.Errorf("insert retrieval session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit retrieval session: %w", err)
	}
	return secret, nil
}

func (r *SecretRepo) GetByRetrievalSession(ctx context.Context, publicID, sessionTokenHash string) (*domain.Secret, error) {
	secret, err := scanSecretRow(r.pool.QueryRow(ctx, `
SELECT s.public_id, s.metadata_token_hash, s.blob_token_hash, s.deletion_token_hash,
    s.encrypted_meta, s.blob_size, s.burn_after_read, s.expires_at, s.created_at, s.retrieved_at,
    s.storage_version, s.status, s.expiration_duration_seconds, s.upload_token_hash,
    s.upload_expires_at, s.chunk_size, s.chunk_count, s.encrypted_total_size, s.completed_at
FROM retrieval_sessions rs
JOIN secrets s ON s.public_id = rs.public_id
WHERE s.public_id = $1
  AND rs.session_token_hash = $2
  AND rs.expires_at > NOW()
  AND s.status = 'active'
  AND s.expires_at > NOW()
`, publicID, sessionTokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("query retrieval session: %w", err)
	}
	return secret, nil
}

func (r *SecretRepo) GetObject(ctx context.Context, publicID, objectKind string, objectIndex int32) (*domain.SecretObject, error) {
	row, err := r.q.GetSecretObject(ctx, dbsqlc.GetSecretObjectParams{
		PublicID:    publicID,
		ObjectKind:  objectKind,
		ObjectIndex: objectIndex,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret object: %w", err)
	}
	object := secretObjectFromRow(row)
	return &object, nil
}

func (r *SecretRepo) CreateObject(ctx context.Context, object *domain.SecretObject) error {
	err := r.q.CreateSecretObject(ctx, dbsqlc.CreateSecretObjectParams{
		PublicID:      object.PublicID,
		ObjectKind:    object.ObjectKind,
		ObjectIndex:   object.ObjectIndex,
		EncryptedSize: object.EncryptedSize,
		Sha256Hex:     object.SHA256Hex,
	})
	if err != nil && isDuplicateKeyError(err) {
		return domain.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert secret object: %w", err)
	}
	return nil
}

func (r *SecretRepo) ListObjects(ctx context.Context, publicID string) ([]domain.SecretObject, error) {
	rows, err := r.q.ListSecretObjects(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("list secret objects: %w", err)
	}
	objects := make([]domain.SecretObject, len(rows))
	for i, row := range rows {
		objects[i] = secretObjectFromRow(row)
	}
	return objects, nil
}

func (r *SecretRepo) CompleteUpload(ctx context.Context, publicID string, blobSize int64, expiresAt time.Time) error {
	n, err := r.q.CompleteChunkedUpload(ctx, dbsqlc.CompleteChunkedUploadParams{
		PublicID:  publicID,
		BlobSize:  blobSize,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("complete chunked upload: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
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

func (r *SecretRepo) DeleteExpired(ctx context.Context, beforeDelete func(publicID string, objects []domain.SecretObject) error) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)

	publicIDs, err := qtx.SelectExpiredForCleanup(ctx)
	if err != nil {
		return 0, fmt.Errorf("select expired: %w", err)
	}

	var deleted int64
	for _, id := range publicIDs {
		rows, err := qtx.ListSecretObjects(ctx, id)
		if err != nil {
			slog.ErrorContext(ctx, "cleanup: list object rows failed, skipping", "public_id", id, "error", err)
			continue
		}
		objects := make([]domain.SecretObject, len(rows))
		for i, row := range rows {
			objects[i] = secretObjectFromRow(row)
		}
		if err := beforeDelete(id, objects); err != nil {
			slog.ErrorContext(ctx, "cleanup: beforeDelete failed, skipping", "public_id", id, "error", err)
			continue
		}
		if _, err := qtx.DeleteSecret(ctx, id); err != nil {
			slog.ErrorContext(ctx, "cleanup: delete row failed", "public_id", id, "error", err)
			continue
		}
		deleted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return deleted, nil
}

func (r *SecretRepo) DeleteExpiredRetrievalSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM retrieval_sessions WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired retrieval sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

func secretFromRow(row dbsqlc.GetSecretByPublicIDRow) *domain.Secret {
	return secretFromValues(
		row.PublicID,
		row.MetadataTokenHash,
		row.BlobTokenHash,
		row.DeletionTokenHash,
		row.EncryptedMeta,
		row.BlobSize,
		row.BurnAfterRead,
		row.ExpiresAt,
		row.CreatedAt,
		row.RetrievedAt,
		row.StorageVersion,
		row.Status,
		row.ExpirationDurationSeconds,
		row.UploadTokenHash,
		row.UploadExpiresAt,
		row.ChunkSize,
		row.ChunkCount,
		row.EncryptedTotalSize,
		row.CompletedAt,
	)
}

func secretFromPendingRow(row dbsqlc.GetPendingUploadByPublicIDRow) *domain.Secret {
	return secretFromValues(
		row.PublicID,
		row.MetadataTokenHash,
		row.BlobTokenHash,
		row.DeletionTokenHash,
		row.EncryptedMeta,
		row.BlobSize,
		row.BurnAfterRead,
		row.ExpiresAt,
		row.CreatedAt,
		row.RetrievedAt,
		row.StorageVersion,
		row.Status,
		row.ExpirationDurationSeconds,
		row.UploadTokenHash,
		row.UploadExpiresAt,
		row.ChunkSize,
		row.ChunkCount,
		row.EncryptedTotalSize,
		row.CompletedAt,
	)
}

type secretRow interface {
	Scan(dest ...any) error
}

func scanSecretRow(row secretRow) (*domain.Secret, error) {
	var secret domain.Secret
	var expiresAt pgtype.Timestamptz
	var createdAt pgtype.Timestamptz
	var retrievedAt pgtype.Timestamptz
	var expirationDurationSeconds pgtype.Int8
	var uploadTokenHash pgtype.Text
	var uploadExpiresAt pgtype.Timestamptz
	var chunkSize pgtype.Int8
	var chunkCount pgtype.Int4
	var encryptedTotalSize pgtype.Int8
	var completedAt pgtype.Timestamptz
	err := row.Scan(
		&secret.PublicID,
		&secret.MetadataTokenHash,
		&secret.BlobTokenHash,
		&secret.DeletionTokenHash,
		&secret.EncryptedMeta,
		&secret.BlobSize,
		&secret.BurnAfterRead,
		&expiresAt,
		&createdAt,
		&retrievedAt,
		&secret.StorageVersion,
		&secret.Status,
		&expirationDurationSeconds,
		&uploadTokenHash,
		&uploadExpiresAt,
		&chunkSize,
		&chunkCount,
		&encryptedTotalSize,
		&completedAt,
	)
	if err != nil {
		return nil, err
	}
	secret.ExpiresAt = expiresAt.Time
	secret.CreatedAt = createdAt.Time
	secret.RetrievedAt = pointerFromTimestamp(retrievedAt)
	secret.ExpirationDurationSeconds = int64FromPg(expirationDurationSeconds)
	secret.UploadTokenHash = textFromPg(uploadTokenHash)
	secret.UploadExpiresAt = pointerFromTimestamp(uploadExpiresAt)
	secret.ChunkSize = int64FromPg(chunkSize)
	secret.ChunkCount = int32FromPg(chunkCount)
	secret.EncryptedTotalSize = int64FromPg(encryptedTotalSize)
	secret.CompletedAt = pointerFromTimestamp(completedAt)
	return &secret, nil
}

func secretFromValues(
	publicID string,
	metadataTokenHash string,
	blobTokenHash string,
	deletionTokenHash string,
	encryptedMeta string,
	blobSize int64,
	burnAfterRead bool,
	expiresAt pgtype.Timestamptz,
	createdAt pgtype.Timestamptz,
	retrievedAt pgtype.Timestamptz,
	storageVersion string,
	status string,
	expirationDurationSeconds pgtype.Int8,
	uploadTokenHash pgtype.Text,
	uploadExpiresAt pgtype.Timestamptz,
	chunkSize pgtype.Int8,
	chunkCount pgtype.Int4,
	encryptedTotalSize pgtype.Int8,
	completedAt pgtype.Timestamptz,
) *domain.Secret {
	return &domain.Secret{
		PublicID:                  publicID,
		MetadataTokenHash:         metadataTokenHash,
		BlobTokenHash:             blobTokenHash,
		DeletionTokenHash:         deletionTokenHash,
		EncryptedMeta:             encryptedMeta,
		BlobSize:                  blobSize,
		BurnAfterRead:             burnAfterRead,
		ExpiresAt:                 expiresAt.Time,
		CreatedAt:                 createdAt.Time,
		RetrievedAt:               pointerFromTimestamp(retrievedAt),
		StorageVersion:            storageVersion,
		Status:                    status,
		ExpirationDurationSeconds: int64FromPg(expirationDurationSeconds),
		UploadTokenHash:           textFromPg(uploadTokenHash),
		UploadExpiresAt:           pointerFromTimestamp(uploadExpiresAt),
		ChunkSize:                 int64FromPg(chunkSize),
		ChunkCount:                int32FromPg(chunkCount),
		EncryptedTotalSize:        int64FromPg(encryptedTotalSize),
		CompletedAt:               pointerFromTimestamp(completedAt),
	}
}

func secretObjectFromRow(row dbsqlc.SecretObject) domain.SecretObject {
	return domain.SecretObject{
		PublicID:      row.PublicID,
		ObjectKind:    row.ObjectKind,
		ObjectIndex:   row.ObjectIndex,
		EncryptedSize: row.EncryptedSize,
		SHA256Hex:     row.Sha256Hex,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func pointerFromTimestamp(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func int64FromPg(t pgtype.Int8) int64 {
	if !t.Valid {
		return 0
	}
	return t.Int64
}

func int32FromPg(t pgtype.Int4) int32 {
	if !t.Valid {
		return 0
	}
	return t.Int32
}

func textFromPg(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func isDuplicateKeyError(err error) bool {
	if err, ok := errors.AsType[*pgconn.PgError](err); ok {
		return err.Code == "23505"
	}
	return false
}
