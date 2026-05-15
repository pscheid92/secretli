package domain

import (
	"context"
	"io"
)

type FileStore interface {
	Put(ctx context.Context, key string, reader io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
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
