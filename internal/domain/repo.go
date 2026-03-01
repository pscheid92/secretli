package domain

import "context"

// SecretRepo defines the persistence contract for secrets.
type SecretRepo interface {
	Create(ctx context.Context, secret *Secret) error
	GetByPublicID(ctx context.Context, publicID string) (*Secret, error)
	GetAndDeleteByPublicID(ctx context.Context, publicID string) (*Secret, error)
	SetRetrievedAt(ctx context.Context, publicID string) error
	Delete(ctx context.Context, publicID string) error
	DeleteExpired(ctx context.Context) (int64, []string, error)
}
