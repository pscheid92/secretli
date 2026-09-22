package domain

import "time"

const (
	UploadSessionStatePending   = "pending"
	UploadSessionStateCompleted = "completed"
	UploadSessionStateAborted   = "aborted"
)

// UploadSession is a pending multipart upload. Once completed or aborted it is
// only a tombstone: the share material (token hashes, encrypted metadata) is
// blanked and the row is purged by cleanup.
type UploadSession struct {
	SessionID         string
	UploadTokenHash   string
	PublicID          string
	StorageKey        string
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
