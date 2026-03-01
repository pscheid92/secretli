package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresUserSecretRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresUserSecretRepo(pool *pgxpool.Pool) *PostgresUserSecretRepo {
	return &PostgresUserSecretRepo{pool: pool}
}

func (r *PostgresUserSecretRepo) LinkSecret(ctx context.Context, userID int64, secretID int64, label string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_secrets (user_id, secret_id, label)
		VALUES ($1, $2, $3)`,
		userID, secretID, label,
	)
	if err != nil {
		return fmt.Errorf("link secret to user: %w", err)
	}
	return nil
}

func (r *PostgresUserSecretRepo) ListByUser(ctx context.Context, userID int64, page, perPage int) ([]SecretSummary, int64, error) {
	// Count total
	var total int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_secrets WHERE user_id = $1`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count user secrets: %w", err)
	}

	if total == 0 {
		return []SecretSummary{}, 0, nil
	}

	offset := (page - 1) * perPage
	rows, err := r.pool.Query(ctx, `
		SELECT s.public_id, us.label, s.secret_type, s.burn_after_read,
			s.password_protected, s.expires_at, s.created_at, s.retrieved_at
		FROM user_secrets us
		JOIN secrets s ON s.id = us.secret_id
		WHERE us.user_id = $1
		ORDER BY s.created_at DESC
		LIMIT $2 OFFSET $3`,
		userID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query user secrets: %w", err)
	}
	defer rows.Close()

	var secrets []SecretSummary
	for rows.Next() {
		var s SecretSummary
		if err := rows.Scan(
			&s.PublicID, &s.Label, &s.SecretType, &s.BurnAfterRead,
			&s.PasswordProtected, &s.ExpiresAt, &s.CreatedAt, &s.RetrievedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan user secret: %w", err)
		}
		secrets = append(secrets, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate user secrets: %w", err)
	}

	return secrets, total, nil
}
