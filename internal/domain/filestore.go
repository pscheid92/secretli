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
