package domain

import (
	"context"
	"io"
)

// FileStore defines the contract for encrypted file storage.
type FileStore interface {
	Put(ctx context.Context, key string, reader io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}
