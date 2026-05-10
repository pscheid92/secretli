package domain

import "context"

type SecretRepo interface {
	Create(ctx context.Context, secret *Secret) error
	GetByPublicID(ctx context.Context, publicID string) (*Secret, error)
	ClaimBurnAfterRead(ctx context.Context, publicID, blobToken string) error
	Delete(ctx context.Context, publicID string) error
	DeleteExpired(ctx context.Context, beforeDelete func(publicID string) error) (int64, error)
}
