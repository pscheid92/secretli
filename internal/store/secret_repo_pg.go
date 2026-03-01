package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store/dbsqlc"
)

var ErrNotFound = errors.New("not found")
var ErrDuplicate = errors.New("duplicate public_id")

type PostgresSecretRepo struct {
	q    *dbsqlc.Queries
	pool *pgxpool.Pool
}

func NewPostgresSecretRepo(pool *pgxpool.Pool) *PostgresSecretRepo {
	return &PostgresSecretRepo{q: dbsqlc.New(pool), pool: pool}
}

func (r *PostgresSecretRepo) Create(ctx context.Context, secret *model.Secret) error {
	id, err := r.q.CreateSecret(ctx, dbsqlc.CreateSecretParams{
		PublicID:           secret.PublicID,
		RetrievalTokenHash: secret.RetrievalTokenHash,
		DeletionTokenHash:  secret.DeletionTokenHash,
		EncryptedData:      textFromPtr(secret.EncryptedData),
		Nonce:              secret.Nonce,
		SecretType:         secret.SecretType,
		StorageKey:         textFromPtr(secret.StorageKey),
		EncryptedFilename:  textFromPtr(secret.EncryptedFilename),
		EncryptedSize:      int8FromPtr(secret.EncryptedSize),
		BurnAfterRead:      secret.BurnAfterRead,
		PasswordProtected:  secret.PasswordProtected,
		ExpiresAt:          pgtype.Timestamptz{Time: secret.ExpiresAt, Valid: true},
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("insert secret: %w", err)
	}
	secret.ID = id
	return nil
}

func (r *PostgresSecretRepo) GetByPublicID(ctx context.Context, publicID string) (*model.Secret, error) {
	row, err := r.q.GetSecretByPublicID(ctx, publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query secret: %w", err)
	}
	return secretFromRow(row), nil
}

func (r *PostgresSecretRepo) GetAndDeleteByPublicID(ctx context.Context, publicID string) (*model.Secret, error) {
	row, err := r.q.GetAndDeleteSecretByPublicID(ctx, publicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delete and return secret: %w", err)
	}
	return secretFromRow(row), nil
}

func (r *PostgresSecretRepo) SetRetrievedAt(ctx context.Context, publicID string) error {
	if err := r.q.SetSecretRetrievedAt(ctx, publicID); err != nil {
		return fmt.Errorf("set retrieved_at: %w", err)
	}
	return nil
}

func (r *PostgresSecretRepo) Delete(ctx context.Context, publicID string) error {
	n, err := r.q.DeleteSecret(ctx, publicID)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresSecretRepo) DeleteExpired(ctx context.Context) (int64, []string, error) {
	keys, err := r.q.DeleteExpiredSecrets(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("delete expired secrets: %w", err)
	}
	var storageKeys []string
	for _, k := range keys {
		if k.Valid {
			storageKeys = append(storageKeys, k.String)
		}
	}
	return int64(len(keys)), storageKeys, nil
}

func secretFromRow(row dbsqlc.Secret) *model.Secret {
	return &model.Secret{
		ID:                 row.ID,
		PublicID:           row.PublicID,
		RetrievalTokenHash: row.RetrievalTokenHash,
		DeletionTokenHash:  row.DeletionTokenHash,
		EncryptedData:      ptrFromText(row.EncryptedData),
		Nonce:              row.Nonce,
		SecretType:         row.SecretType,
		StorageKey:         ptrFromText(row.StorageKey),
		EncryptedFilename:  ptrFromText(row.EncryptedFilename),
		EncryptedSize:      ptrFromInt8(row.EncryptedSize),
		BurnAfterRead:      row.BurnAfterRead,
		PasswordProtected:  row.PasswordProtected,
		ExpiresAt:          row.ExpiresAt.Time,
		CreatedAt:          row.CreatedAt.Time,
		RetrievedAt:        ptrFromTimestamptz(row.RetrievedAt),
	}
}

// --- pgtype conversion helpers ---

func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func ptrFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func int8FromPtr(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func ptrFromInt8(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

func ptrFromTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func isDuplicateKeyError(err error) bool {
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
