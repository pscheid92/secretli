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
		PublicID:       secret.PublicID,
		RetrievalToken: secret.RetrievalToken,
		DeletionToken:  secret.DeletionToken,
		EncryptedMeta:  secret.EncryptedMeta,
		BlobSize:       secret.BlobSize,
		BurnAfterRead:  secret.BurnAfterRead,
		ExpiresAt:      pgtype.Timestamptz{Time: secret.ExpiresAt, Valid: true},
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

func secretFromRow(row dbsqlc.Secret) *domain.Secret {
	return &domain.Secret{
		PublicID:       row.PublicID,
		RetrievalToken: row.RetrievalToken,
		DeletionToken:  row.DeletionToken,
		EncryptedMeta:  row.EncryptedMeta,
		BlobSize:       row.BlobSize,
		BurnAfterRead:  row.BurnAfterRead,
		ExpiresAt:      row.ExpiresAt.Time,
		CreatedAt:      row.CreatedAt.Time,
		RetrievedAt:    pointerFromTimestamp(row.RetrievedAt),
	}
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
