package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pscheid92/secretli/internal/adapter/postgres/dbsqlc"
	"github.com/pscheid92/secretli/internal/domain"
)

type SecretRepo struct {
	q    *dbsqlc.Queries
	pool *pgxpool.Pool
}

func NewSecretRepo(pool *pgxpool.Pool) *SecretRepo {
	return &SecretRepo{q: dbsqlc.New(pool), pool: pool}
}

func (r *SecretRepo) Create(ctx context.Context, secret *domain.Secret) error {
	err := r.q.CreateSecret(ctx, dbsqlc.CreateSecretParams{
		PublicID:          secret.PublicID,
		RetrievalToken:    secret.RetrievalToken,
		DeletionToken:     secret.DeletionToken,
		EncryptedMeta:     secret.EncryptedMeta,
		BlobSize:          secret.BlobSize,
		BurnAfterRead:     secret.BurnAfterRead,

		ExpiresAt:         pgtype.Timestamptz{Time: secret.ExpiresAt, Valid: true},
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			return domain.ErrDuplicate
		}
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

func (r *SecretRepo) GetAndDeleteByPublicID(ctx context.Context, publicID string) (*domain.Secret, error) {
	row, err := r.q.GetAndDeleteSecretByPublicID(ctx, publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delete and return secret: %w", err)
	}
	return secretFromRow(row), nil
}

func (r *SecretRepo) SetRetrievedAt(ctx context.Context, publicID string) error {
	if err := r.q.SetSecretRetrievedAt(ctx, publicID); err != nil {
		return fmt.Errorf("set retrieved_at: %w", err)
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

func (r *SecretRepo) DeleteExpired(ctx context.Context) (int64, []string, error) {
	publicIDs, err := r.q.DeleteExpiredSecrets(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("delete expired secrets: %w", err)
	}
	return int64(len(publicIDs)), publicIDs, nil
}

func secretFromRow(row dbsqlc.Secret) *domain.Secret {
	return &domain.Secret{
		PublicID:          row.PublicID,
		RetrievalToken:    row.RetrievalToken,
		DeletionToken:     row.DeletionToken,
		EncryptedMeta:     row.EncryptedMeta,
		BlobSize:          row.BlobSize,
		BurnAfterRead:     row.BurnAfterRead,

		ExpiresAt:         row.ExpiresAt.Time,
		CreatedAt:         row.CreatedAt.Time,
		RetrievedAt:       ptrFromTimestamptz(row.RetrievedAt),
	}
}

func ptrFromTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func isDuplicateKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
