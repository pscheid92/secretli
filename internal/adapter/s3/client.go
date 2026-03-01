package s3

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pscheid92/secretli/internal/platform/config"
)

type Client struct {
	client *minio.Client
	bucket string
}

func NewClient(cfg config.S3Config) (*Client, error) {
	params := minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	client, err := minio.New(cfg.Endpoint, &params)
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	exists, err := client.BucketExists(context.Background(), cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("bucket %q does not exist", cfg.Bucket)
	}

	return &Client{client: client, bucket: cfg.Bucket}, nil
}

func (s *Client) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

func (s *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	return obj, nil
}

func (s *Client) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}
