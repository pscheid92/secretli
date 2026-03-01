package storage_test

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testBucket = "test-bucket"

func setupMinIO(t *testing.T) *storage.S3Client {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping integration test: docker not found in PATH")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("skipping integration test: docker not available: %v", err)
	}

	ctx := context.Background()

	container, err := tcminio.Run(ctx,
		"minio/minio:latest",
		tcminio.WithUsername("minioadmin"),
		tcminio.WithPassword("minioadmin"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/minio/health/live").
				WithPort("9000/tcp").
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start minio container: %v", err)
	}

	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate minio container: %v", err)
		}
	})

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("get minio connection string: %v", err)
	}

	// Create the test bucket using the minio client directly
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("create minio admin client: %v", err)
	}
	if err := mc.MakeBucket(ctx, testBucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("create test bucket: %v", err)
	}

	client, err := storage.NewS3Client(endpoint, testBucket, "minioadmin", "minioadmin", "", false)
	if err != nil {
		t.Fatalf("create S3Client: %v", err)
	}

	return client
}

func TestS3Client_PutAndGet(t *testing.T) {
	client := setupMinIO(t)
	ctx := context.Background()

	data := []byte("hello, minio integration test!")
	err := client.Put(ctx, "test-key", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	reader, err := client.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Errorf("Get returned %q, want %q", got, data)
	}
}

func TestS3Client_Delete(t *testing.T) {
	client := setupMinIO(t)
	ctx := context.Background()

	data := []byte("to be deleted")
	if err := client.Put(ctx, "del-key", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := client.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete — minio returns an error when reading the object
	reader, err := client.Get(ctx, "del-key")
	if err != nil {
		// Some versions return error on Get itself
		return
	}
	defer reader.Close()
	_, err = io.ReadAll(reader)
	if err == nil {
		t.Error("expected error reading deleted object, got nil")
	}
}

func TestS3Client_PutOverwrite(t *testing.T) {
	client := setupMinIO(t)
	ctx := context.Background()

	data1 := []byte("version 1")
	if err := client.Put(ctx, "overwrite-key", bytes.NewReader(data1), int64(len(data1))); err != nil {
		t.Fatalf("Put v1: %v", err)
	}

	data2 := []byte("version 2")
	if err := client.Put(ctx, "overwrite-key", bytes.NewReader(data2), int64(len(data2))); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	reader, err := client.Get(ctx, "overwrite-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, data2) {
		t.Errorf("Get returned %q, want %q", got, data2)
	}
}

func TestS3Client_LargeFile(t *testing.T) {
	client := setupMinIO(t)
	ctx := context.Background()

	// 1MB file
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := client.Put(ctx, "large-key", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	reader, err := client.Get(ctx, "large-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(got) != len(data) {
		t.Errorf("got %d bytes, want %d", len(got), len(data))
	}
	if !bytes.Equal(got, data) {
		t.Error("large file content mismatch")
	}
}

func TestNewS3Client_BucketNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping integration test: docker not found in PATH")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("skipping integration test: docker not available: %v", err)
	}

	ctx := context.Background()

	container, err := tcminio.Run(ctx,
		"minio/minio:latest",
		tcminio.WithUsername("minioadmin"),
		tcminio.WithPassword("minioadmin"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/minio/health/live").
				WithPort("9000/tcp").
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start minio container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate minio container: %v", err)
		}
	})

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	_, err = storage.NewS3Client(endpoint, "nonexistent-bucket", "minioadmin", "minioadmin", "", false)
	if err == nil {
		t.Fatal("expected error for nonexistent bucket, got nil")
	}
}
