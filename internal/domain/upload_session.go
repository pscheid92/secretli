package domain

import "time"

const (
	UploadSessionStatePending   = "pending"
	UploadSessionStateCompleted = "completed"
	UploadSessionStateAborted   = "aborted"
)

type UploadSession struct {
	SessionID         string
	UploadTokenHash   string
	PublicID          string
	S3UploadID        string
	BlobSize          int64
	MetadataTokenHash string
	BlobTokenHash     string
	DeletionTokenHash string
	EncryptedMeta     string
	BurnAfterRead     bool
	SecretExpiresAt   time.Time
	UploadExpiresAt   time.Time
	State             string
	CreatedAt         time.Time
	CompletedAt       *time.Time
	AbortedAt         *time.Time
}

type UploadPart struct {
	SessionID  string
	PartNumber int
	Offset     int64
	Size       int64
	SHA256     string
	ETag       string
	CreatedAt  time.Time
}
