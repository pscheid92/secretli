package domain

import "time"

const (
	StorageVersionSingle  = "single-v1"
	StorageVersionChunked = "chunked-v1"

	SecretStatusPending = "pending"
	SecretStatusActive  = "active"

	ObjectKindManifest  = "manifest"
	ObjectKindChunk     = "chunk"
	ManifestObjectIndex = -1
)

type Secret struct {
	PublicID                  string     `json:"public_id"`
	MetadataTokenHash         string     `json:"-"`
	BlobTokenHash             string     `json:"-"`
	DeletionTokenHash         string     `json:"-"`
	EncryptedMeta             string     `json:"encrypted_meta"`
	BlobSize                  int64      `json:"blob_size"`
	BurnAfterRead             bool       `json:"burn_after_read"`
	ExpiresAt                 time.Time  `json:"expires_at"`
	CreatedAt                 time.Time  `json:"created_at"`
	RetrievedAt               *time.Time `json:"-"`
	StorageVersion            string     `json:"storage_version"`
	Status                    string     `json:"-"`
	ExpirationDurationSeconds int64      `json:"-"`
	UploadTokenHash           string     `json:"-"`
	UploadExpiresAt           *time.Time `json:"-"`
	ChunkSize                 int64      `json:"-"`
	ChunkCount                int32      `json:"-"`
	EncryptedTotalSize        int64      `json:"-"`
	CompletedAt               *time.Time `json:"-"`
}

type CreateSecretRequest struct {
	PublicID      string `json:"public_id" form:"public_id" validate:"required,public_id"`
	MetadataToken string `json:"metadata_token" form:"metadata_token" validate:"required,secret_token"`
	BlobToken     string `json:"blob_token" form:"blob_token" validate:"required,secret_token"`
	DeletionToken string `json:"deletion_token" form:"deletion_token" validate:"required,secret_token"`
	EncryptedMeta string `json:"encrypted_meta" form:"encrypted_meta" validate:"required,encrypted_meta"`
	Expiration    string `json:"expiration" form:"expiration" validate:"required,expiration"`
	BurnAfterRead bool   `json:"burn_after_read" form:"burn_after_read"`
}

type SecretMetadataResponse struct {
	EncryptedMeta  string `json:"encrypted_meta"`
	BlobSize       int64  `json:"blob_size"`
	BurnAfterRead  bool   `json:"burn_after_read"`
	ExpiresAt      string `json:"expires_at"`
	CreatedAt      string `json:"created_at"`
	StorageVersion string `json:"storage_version"`
}

type CreateUploadRequest struct {
	PublicID           string `json:"public_id" validate:"required,public_id"`
	MetadataToken      string `json:"metadata_token" validate:"required,secret_token"`
	BlobToken          string `json:"blob_token" validate:"required,secret_token"`
	DeletionToken      string `json:"deletion_token" validate:"required,secret_token"`
	EncryptedMeta      string `json:"encrypted_meta" validate:"required,encrypted_meta"`
	Expiration         string `json:"expiration" validate:"required,expiration"`
	BurnAfterRead      bool   `json:"burn_after_read"`
	ChunkSize          int64  `json:"chunk_size" validate:"required,gt=0"`
	ChunkCount         int32  `json:"chunk_count" validate:"gte=0"`
	EncryptedTotalSize int64  `json:"encrypted_total_size" validate:"required,gt=0"`
}

type SecretObject struct {
	PublicID      string
	ObjectKind    string
	ObjectIndex   int32
	EncryptedSize int64
	SHA256Hex     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
