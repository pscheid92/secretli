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
    encrypted_meta, blob_size, burn_after_read, expires_at, created_at, retrieved_at
FROM secrets
WHERE public_id = $1 AND expires_at > NOW()
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
    s.encrypted_meta, s.blob_size, s.burn_after_read, s.expires_at, s.created_at, s.retrieved_at
FROM retrieval_sessions rs
JOIN secrets s ON s.public_id = rs.public_id
WHERE s.public_id = $1
  AND rs.session_token_hash = $2
  AND rs.expires_at > NOW()
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

func (r *SecretRepo) DeleteExpired(ctx context.Context, beforeDelete func(publicID string) error) (int64, error) {
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
		if err := beforeDelete(id); err != nil {
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
	}
}

type secretRow interface {
	Scan(dest ...any) error
}

func scanSecretRow(row secretRow) (*domain.Secret, error) {
	var secret domain.Secret
	var expiresAt pgtype.Timestamptz
	var createdAt pgtype.Timestamptz
	var retrievedAt pgtype.Timestamptz
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
	)
	if err != nil {
		return nil, err
	}
	secret.ExpiresAt = expiresAt.Time
	secret.CreatedAt = createdAt.Time
	secret.RetrievedAt = pointerFromTimestamp(retrievedAt)
	return &secret, nil
}

func pointerFromTimestamp(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func isDuplicateKeyError(err error) bool {
	if err, ok := errors.AsType[*pgconn.PgError](err); ok {
		return err.Code == "23505"
	}
	return false
}
