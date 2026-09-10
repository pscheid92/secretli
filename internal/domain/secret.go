package domain

import "time"

type Secret struct {
	PublicID          string     `json:"public_id"`
	MetadataTokenHash string     `json:"-"`
	BlobTokenHash     string     `json:"-"`
	DeletionTokenHash string     `json:"-"`
	EncryptedMeta     string     `json:"encrypted_meta"`
	BlobSize          int64      `json:"blob_size"`
	BurnAfterRead     bool       `json:"burn_after_read"`
	ExpiresAt         time.Time  `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	RetrievedAt       *time.Time `json:"-"`
}

type SecretMetadataResponse struct {
	EncryptedMeta string `json:"encrypted_meta"`
	BlobSize      int64  `json:"blob_size"`
	BurnAfterRead bool   `json:"burn_after_read"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
}
