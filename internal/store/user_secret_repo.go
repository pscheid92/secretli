package store

import (
	"context"
	"time"
)

type SecretSummary struct {
	PublicID          string     `json:"public_id"`
	Label             string     `json:"label"`
	SecretType        string     `json:"secret_type"`
	BurnAfterRead     bool       `json:"burn_after_read"`
	PasswordProtected bool       `json:"password_protected"`
	ExpiresAt         time.Time  `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	RetrievedAt       *time.Time `json:"retrieved_at"`
}

type UserSecretRepo interface {
	LinkSecret(ctx context.Context, userID int64, secretID int64, label string) error
	ListByUser(ctx context.Context, userID int64, page, perPage int) ([]SecretSummary, int64, error)
}
