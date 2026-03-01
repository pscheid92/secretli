package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/model"
)

type PostgresSessionRepo struct {
	pool      *pgxpool.Pool
	maxAge    time.Duration
}

func NewPostgresSessionRepo(pool *pgxpool.Pool, maxAge time.Duration) *PostgresSessionRepo {
	return &PostgresSessionRepo{pool: pool, maxAge: maxAge}
}

func (r *PostgresSessionRepo) Create(ctx context.Context, userID int64) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	sessionID := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(r.maxAge)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, expires_at)
		VALUES ($1, $2, $3)`,
		sessionID, userID, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return sessionID, nil
}

func (r *PostgresSessionRepo) GetByIDWithUser(ctx context.Context, sessionID string) (*model.User, error) {
	var u model.User
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.password_hash, u.display_name, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.expires_at > NOW()`,
		sessionID,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query session with user: %w", err)
	}
	return &u, nil
}

func (r *PostgresSessionRepo) Delete(ctx context.Context, sessionID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *PostgresSessionRepo) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
