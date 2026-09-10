package domain

import (
	"context"
	"io"
)

// FileStore is the object storage a secret's encrypted blob lives in. Blobs are
// always written through the multipart API and always read as ranges, so there
// is deliberately no whole-object put or get.
type FileStore interface {
	GetRange(ctx context.Context, key string, start, end int64) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type CompletedPart struct {
	PartNumber int
	ETag       string
}

type MultipartFileStore interface {
	FileStore
	CreateMultipartUpload(ctx context.Context, key string) (uploadID string, err error)
	UploadPart(ctx context.Context, key, uploadID string, partNumber int, reader io.Reader, size int64) (etag string, err error)
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error
	AbortMultipartUpload(ctx context.Context, key, uploadID string) error
}
