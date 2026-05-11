package s3

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	platformconfig "github.com/pscheid92/secretli/internal/platform/config"
)

type Client struct {
	client *s3.Client
	bucket string
}

func NewClient(cfg platformconfig.S3Config) (*Client, error) {
	awsCfg := aws.Config{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointURL(cfg.Endpoint, cfg.UseSSL))
		o.UsePathStyle = true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.Bucket),
	}); err != nil {
		return nil, fmt.Errorf("bucket %q does not exist", cfg.Bucket)
	}

	return &Client{client: client, bucket: cfg.Bucket}, nil
}

func (s *Client) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
	}); err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

func (s *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	return obj.Body, nil
}

func (s *Client) GetRange(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	if start < 0 || end < start {
		return nil, fmt.Errorf("invalid object range %d-%d", start, end)
	}
	obj, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
	})
	if err != nil {
		return nil, fmt.Errorf("get object range %q %d-%d: %w", key, start, end, err)
	}
	return obj.Body, nil
}

func (s *Client) Delete(ctx context.Context, key string) error {
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}

func endpointURL(endpoint string, useSSL bool) string {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return scheme + "://" + endpoint
}
