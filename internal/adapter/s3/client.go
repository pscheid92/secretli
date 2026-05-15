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
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pscheid92/secretli/internal/domain"
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

func (s *Client) CreateMultipartUpload(ctx context.Context, key string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("create multipart upload %q: %w", key, err)
	}
	if out.UploadId == nil || *out.UploadId == "" {
		return "", fmt.Errorf("create multipart upload %q: missing upload id", key)
	}
	return *out.UploadId, nil
}

func (s *Client) UploadPart(ctx context.Context, key, uploadID string, partNumber int, reader io.Reader, size int64) (string, error) {
	out, err := s.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		UploadId:      aws.String(uploadID),
		PartNumber:    aws.Int32(int32(partNumber)),
		Body:          reader,
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return "", fmt.Errorf("upload part %q #%d: %w", key, partNumber, err)
	}
	if out.ETag == nil || *out.ETag == "" {
		return "", fmt.Errorf("upload part %q #%d: missing etag", key, partNumber)
	}
	return *out.ETag, nil
}

func (s *Client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []domain.CompletedPart) error {
	completed := make([]types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completed = append(completed, types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(int32(part.PartNumber)),
		})
	}

	if _, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completed,
		},
	}); err != nil {
		return fmt.Errorf("complete multipart upload %q: %w", key, err)
	}
	return nil
}

func (s *Client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	if _, err := s.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}); err != nil {
		return fmt.Errorf("abort multipart upload %q: %w", key, err)
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
