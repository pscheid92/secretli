package domain

import "time"

type Secret struct {
	PublicID       string     `json:"public_id"`
	RetrievalToken string     `json:"-"`
	DeletionToken  string     `json:"-"`
	EncryptedMeta  string     `json:"encrypted_meta"`
	BlobSize       int64      `json:"blob_size"`
	BurnAfterRead  bool       `json:"burn_after_read"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
	RetrievedAt    *time.Time `json:"-"`
}

type CreateSecretRequest struct {
	PublicID       string `json:"public_id" validate:"required"`
	RetrievalToken string `json:"retrieval_token" validate:"required"`
	DeletionToken  string `json:"deletion_token" validate:"required"`
	EncryptedMeta  string `json:"encrypted_meta" validate:"required"`
	Expiration     string `json:"expiration" validate:"required,expiration"`
	BurnAfterRead  bool   `json:"burn_after_read"`
}

type SecretMetadataResponse struct {
	EncryptedMeta string `json:"encrypted_meta"`
	BlobSize      int64  `json:"blob_size"`
	BurnAfterRead bool   `json:"burn_after_read"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
}
