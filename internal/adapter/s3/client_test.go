package s3_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/adapter/s3"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testBucket = "test-bucket"

type seaweedFSTestEnv struct {
	client    *s3.Client
	container testcontainers.Container
	endpoint  string
}

var (
	seaweedDockerOnce sync.Once
	seaweedDockerErr  error

	seaweedOnce sync.Once
	seaweedEnv  seaweedFSTestEnv
	seaweedErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if seaweedEnv.container != nil {
		_ = seaweedEnv.container.Terminate(context.Background())
	}
	os.Exit(code)
}

func setupSeaweedFS(t *testing.T) *s3.Client {
	t.Helper()

	return setupSeaweedFSEnv(t).client
}

func setupSeaweedFSEnv(t *testing.T) seaweedFSTestEnv {
	t.Helper()
	requireDocker(t)
	seaweedOnce.Do(startSeaweedFS)
	if seaweedErr != nil {
		t.Fatalf("setup seaweedfs: %v", seaweedErr)
	}
	return seaweedEnv
}

func requireDocker(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	seaweedDockerOnce.Do(func() {
		if _, err := exec.LookPath("docker"); err != nil {
			seaweedDockerErr = err
			return
		}
		seaweedDockerErr = exec.Command("docker", "info").Run()
	})
	if seaweedDockerErr != nil {
		t.Skipf("skipping integration test: docker unavailable: %v", seaweedDockerErr)
	}
}

func startSeaweedFS() {
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "chrislusf/seaweedfs:latest",
			ExposedPorts: []string{"8333/tcp"},
			Cmd:          []string{"server", "-s3", "-dir=/data"},
			WaitingFor: wait.ForListeningPort("8333/tcp").
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		seaweedErr = err
		return
	}
	seaweedEnv.container = container

	host, err := container.Host(ctx)
	if err != nil {
		seaweedErr = err
		return
	}
	port, err := container.MappedPort(ctx, "8333/tcp")
	if err != nil {
		seaweedErr = err
		return
	}
	endpoint := net.JoinHostPort(host, port.Port())
	seaweedEnv.endpoint = endpoint

	if err := createBucket(endpoint, testBucket); err != nil {
		seaweedErr = err
		return
	}

	client, err := s3.NewClient(config.S3Config{
		Endpoint:  endpoint,
		Bucket:    testBucket,
		AccessKey: "admin",
		SecretKey: "admin",
		UseSSL:    false,
		Region:    "us-east-1",
	})
	if err != nil {
		seaweedErr = err
		return
	}

	seaweedEnv.client = client
}

func createBucket(endpoint, bucket string) error {
	url := "http://" + endpoint + "/" + bucket
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodPut, url, nil)
		if err != nil {
			return err
		}
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				return nil
			}
			lastErr = io.ErrUnexpectedEOF
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return lastErr
}

func TestS3Client_PutAndGet(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	data := []byte("hello, seaweedfs integration test!")
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
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	data := []byte("to be deleted")
	if err := client.Put(ctx, "del-key", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := client.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete may fail either when opening the object or reading it,
	// depending on the S3-compatible server implementation.
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
	t.Parallel()
	client := setupSeaweedFS(t)
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
	t.Parallel()
	client := setupSeaweedFS(t)
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

func TestS3Client_GetRange(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i % 251)
	}

	if err := client.Put(ctx, "range-key", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	tests := []struct {
		name       string
		start, end int64
	}{
		{name: "first byte", start: 0, end: 0},
		{name: "prefix", start: 0, end: 1023},
		{name: "middle", start: 100_000, end: 101_000},
		{name: "last byte", start: int64(len(data) - 1), end: int64(len(data) - 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := client.GetRange(ctx, "range-key", tt.start, tt.end)
			if err != nil {
				t.Fatalf("GetRange: %v", err)
			}
			defer reader.Close()

			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			want := data[tt.start : tt.end+1]
			if !bytes.Equal(got, want) {
				t.Fatalf("range bytes mismatch: got %d bytes, want %d bytes", len(got), len(want))
			}
		})
	}
}

func TestS3Client_GetRangeRejectsMalformedRange(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	_, err := client.GetRange(context.Background(), "range-key", 5, 4)
	if err == nil {
		t.Fatal("expected malformed range error")
	}
}

func TestNewS3Client_BucketNotFound(t *testing.T) {
	t.Parallel()
	env := setupSeaweedFSEnv(t)

	_, err := s3.NewClient(config.S3Config{
		Endpoint:  env.endpoint,
		Bucket:    "nonexistent-bucket",
		AccessKey: "admin",
		SecretKey: "admin",
		UseSSL:    false,
		Region:    "us-east-1",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent bucket, got nil")
	}
}

func TestS3Client_MultipartRoundTrip(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	const key = "multipart/roundtrip"
	partOne := bytes.Repeat([]byte("a"), 5*1024*1024)
	partTwo := []byte("tail-bytes")

	uploadID, err := client.CreateMultipartUpload(ctx, key)
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}
	etagOne, err := client.UploadPart(ctx, key, uploadID, 1, bytes.NewReader(partOne), int64(len(partOne)))
	if err != nil {
		t.Fatalf("UploadPart 1: %v", err)
	}
	etagTwo, err := client.UploadPart(ctx, key, uploadID, 2, bytes.NewReader(partTwo), int64(len(partTwo)))
	if err != nil {
		t.Fatalf("UploadPart 2: %v", err)
	}
	if err := client.CompleteMultipartUpload(ctx, key, uploadID, []domain.CompletedPart{
		{PartNumber: 1, ETag: etagOne},
		{PartNumber: 2, ETag: etagTwo},
	}); err != nil {
		t.Fatalf("CompleteMultipartUpload: %v", err)
	}

	reader, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := append(append([]byte(nil), partOne...), partTwo...)
	if !bytes.Equal(got, want) {
		t.Fatalf("assembled object has %d bytes, want %d", len(got), len(want))
	}

	// Aborting a completed (no longer existing) upload is a no-op.
	if err := client.AbortMultipartUpload(ctx, key, uploadID); err != nil {
		t.Fatalf("AbortMultipartUpload after complete: %v", err)
	}
}

func TestS3Client_AbortMultipartUploadIsIdempotent(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	const key = "multipart/abort"
	uploadID, err := client.CreateMultipartUpload(ctx, key)
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}
	if _, err := client.UploadPart(ctx, key, uploadID, 1, bytes.NewReader([]byte("part")), 4); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}

	if err := client.AbortMultipartUpload(ctx, key, uploadID); err != nil {
		t.Fatalf("first AbortMultipartUpload: %v", err)
	}
	if err := client.AbortMultipartUpload(ctx, key, uploadID); err != nil {
		t.Fatalf("second AbortMultipartUpload should succeed: %v", err)
	}
	if err := client.AbortMultipartUpload(ctx, key, "never-existed"); err != nil {
		t.Fatalf("AbortMultipartUpload of unknown upload should succeed: %v", err)
	}

	err = client.CompleteMultipartUpload(ctx, key, uploadID, []domain.CompletedPart{{PartNumber: 1, ETag: "x"}})
	if !errors.Is(err, domain.ErrUploadNotFound) {
		t.Fatalf("CompleteMultipartUpload after abort error = %v, want ErrUploadNotFound", err)
	}
}

func TestS3Client_CompleteMultipartUploadRejectsStaleETag(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	const key = "multipart/stale-etag"
	uploadID, err := client.CreateMultipartUpload(ctx, key)
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}
	t.Cleanup(func() { _ = client.AbortMultipartUpload(context.Background(), key, uploadID) })
	if _, err := client.UploadPart(ctx, key, uploadID, 1, bytes.NewReader([]byte("part")), 4); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}

	err = client.CompleteMultipartUpload(ctx, key, uploadID, []domain.CompletedPart{{PartNumber: 1, ETag: "\"deadbeef\""}})
	if !errors.Is(err, domain.ErrInvalidParts) {
		t.Fatalf("CompleteMultipartUpload with stale etag error = %v, want ErrInvalidParts", err)
	}
}

func TestS3Client_MultipartSinglePart(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	// Small bundles are uploaded as one final part well below the S3 minimum
	// for non-final parts; the backend must accept that.
	const key = "multipart/single-part"
	payload := []byte("tiny single-part bundle")

	uploadID, err := client.CreateMultipartUpload(ctx, key)
	if err != nil {
		t.Fatalf("CreateMultipartUpload: %v", err)
	}
	etag, err := client.UploadPart(ctx, key, uploadID, 1, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("UploadPart: %v", err)
	}
	if err := client.CompleteMultipartUpload(ctx, key, uploadID, []domain.CompletedPart{{PartNumber: 1, ETag: etag}}); err != nil {
		t.Fatalf("CompleteMultipartUpload: %v", err)
	}

	reader, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("object = %q, want %q", got, payload)
	}
}
