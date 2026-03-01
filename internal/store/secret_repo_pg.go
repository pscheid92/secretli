package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/model"
)

var ErrNotFound = errors.New("not found")
var ErrDuplicate = errors.New("duplicate public_id")

type PostgresSecretRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresSecretRepo(pool *pgxpool.Pool) *PostgresSecretRepo {
	return &PostgresSecretRepo{pool: pool}
}

func (r *PostgresSecretRepo) Create(ctx context.Context, secret *model.Secret) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO secrets (
			public_id, retrieval_token_hash, deletion_token_hash,
			encrypted_data, nonce, secret_type,
			storage_key, encrypted_filename, encrypted_size,
			burn_after_read, password_protected, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		secret.PublicID,
		secret.RetrievalTokenHash,
		secret.DeletionTokenHash,
		secret.EncryptedData,
		secret.Nonce,
		secret.SecretType,
		secret.StorageKey,
		secret.EncryptedFilename,
		secret.EncryptedSize,
		secret.BurnAfterRead,
		secret.PasswordProtected,
		secret.ExpiresAt,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("insert secret: %w", err)
	}
	return nil
}

func (r *PostgresSecretRepo) GetByPublicID(ctx context.Context, publicID string) (*model.Secret, error) {
	var s model.Secret
	err := r.pool.QueryRow(ctx, `
		SELECT id, public_id, retrieval_token_hash, deletion_token_hash,
			encrypted_data, nonce, secret_type,
			storage_key, encrypted_filename, encrypted_size,
			burn_after_read, password_protected,
			expires_at, created_at, retrieved_at
		FROM secrets
		WHERE public_id = $1 AND expires_at > NOW()`,
		publicID,
	).Scan(
		&s.ID, &s.PublicID, &s.RetrievalTokenHash, &s.DeletionTokenHash,
		&s.EncryptedData, &s.Nonce, &s.SecretType,
		&s.StorageKey, &s.EncryptedFilename, &s.EncryptedSize,
		&s.BurnAfterRead, &s.PasswordProtected,
		&s.ExpiresAt, &s.CreatedAt, &s.RetrievedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret: %w", err)
	}
	return &s, nil
}

func (r *PostgresSecretRepo) GetAndDeleteByPublicID(ctx context.Context, publicID string) (*model.Secret, error) {
	var s model.Secret
	err := r.pool.QueryRow(ctx, `
		DELETE FROM secrets
		WHERE public_id = $1 AND expires_at > NOW()
		RETURNING id, public_id, retrieval_token_hash, deletion_token_hash,
			encrypted_data, nonce, secret_type,
			storage_key, encrypted_filename, encrypted_size,
			burn_after_read, password_protected,
			expires_at, created_at, retrieved_at`,
		publicID,
	).Scan(
		&s.ID, &s.PublicID, &s.RetrievalTokenHash, &s.DeletionTokenHash,
		&s.EncryptedData, &s.Nonce, &s.SecretType,
		&s.StorageKey, &s.EncryptedFilename, &s.EncryptedSize,
		&s.BurnAfterRead, &s.PasswordProtected,
		&s.ExpiresAt, &s.CreatedAt, &s.RetrievedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delete and return secret: %w", err)
	}
	return &s, nil
}

func (r *PostgresSecretRepo) SetRetrievedAt(ctx context.Context, publicID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE secrets SET retrieved_at = NOW()
		WHERE public_id = $1 AND retrieved_at IS NULL`,
		publicID,
	)
	if err != nil {
		return fmt.Errorf("set retrieved_at: %w", err)
	}
	return nil
}

func (r *PostgresSecretRepo) Delete(ctx context.Context, publicID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM secrets WHERE public_id = $1`, publicID)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresSecretRepo) DeleteExpired(ctx context.Context) (int64, []string, error) {
	rows, err := r.pool.Query(ctx, `
		DELETE FROM secrets WHERE expires_at < NOW()
		RETURNING storage_key`)
	if err != nil {
		return 0, nil, fmt.Errorf("delete expired secrets: %w", err)
	}
	defer rows.Close()

	var count int64
	var storageKeys []string
	for rows.Next() {
		var key *string
		if err := rows.Scan(&key); err != nil {
			return count, storageKeys, fmt.Errorf("scan storage key: %w", err)
		}
		count++
		if key != nil {
			storageKeys = append(storageKeys, *key)
		}
	}
	if err := rows.Err(); err != nil {
		return count, storageKeys, fmt.Errorf("iterate expired secrets: %w", err)
	}
	return count, storageKeys, nil
}

func isDuplicateKeyError(err error) bool {
	// pgx wraps postgres errors; check for unique_violation (23505)
	return err != nil && contains(err.Error(), "23505")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
