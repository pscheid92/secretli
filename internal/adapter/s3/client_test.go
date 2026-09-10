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

// putTestObject writes an object the way production does: one multipart upload
// with a single final part. There is no whole-object put in the FileStore API.
func putTestObject(t *testing.T, client *s3.Client, key string, data []byte) {
	t.Helper()
	ctx := context.Background()
	uploadID, err := client.CreateMultipartUpload(ctx, key)
	if err != nil {
		t.Fatalf("CreateMultipartUpload %q: %v", key, err)
	}
	etag, err := client.UploadPart(ctx, key, uploadID, 1, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("UploadPart %q: %v", key, err)
	}
	if err := client.CompleteMultipartUpload(ctx, key, uploadID, []domain.CompletedPart{{PartNumber: 1, ETag: etag}}); err != nil {
		t.Fatalf("CompleteMultipartUpload %q: %v", key, err)
	}
}

// readTestObject reads a whole object back through the range API.
func readTestObject(t *testing.T, client *s3.Client, key string, size int) []byte {
	t.Helper()
	reader, err := client.GetRange(context.Background(), key, 0, int64(size-1))
	if err != nil {
		t.Fatalf("GetRange %q: %v", key, err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll %q: %v", key, err)
	}
	return got
}

func TestS3Client_WriteAndReadRange(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)

	data := []byte("hello, seaweedfs integration test!")
	putTestObject(t, client, "test-key", data)

	if got := readTestObject(t, client, "test-key", len(data)); !bytes.Equal(got, data) {
		t.Errorf("read %q, want %q", got, data)
	}
}

func TestS3Client_Delete(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)
	ctx := context.Background()

	data := []byte("to be deleted")
	putTestObject(t, client, "del-key", data)

	if err := client.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// A read after delete may fail when opening the object or when reading it,
	// depending on the S3-compatible server implementation.
	reader, err := client.GetRange(ctx, "del-key", 0, int64(len(data)-1))
	if err != nil {
		return
	}
	defer reader.Close()
	if _, err := io.ReadAll(reader); err == nil {
		t.Error("expected error reading deleted object, got nil")
	}
}

func TestS3Client_Overwrite(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)

	putTestObject(t, client, "overwrite-key", []byte("version 1"))
	data2 := []byte("version 2")
	putTestObject(t, client, "overwrite-key", data2)

	if got := readTestObject(t, client, "overwrite-key", len(data2)); !bytes.Equal(got, data2) {
		t.Errorf("read %q, want %q", got, data2)
	}
}

func TestS3Client_LargeFile(t *testing.T) {
	t.Parallel()
	client := setupSeaweedFS(t)

	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i % 256)
	}
	putTestObject(t, client, "large-key", data)

	got := readTestObject(t, client, "large-key", len(data))
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

	putTestObject(t, client, "range-key", data)

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

	want := append(append([]byte(nil), partOne...), partTwo...)
	if got := readTestObject(t, client, key, len(want)); !bytes.Equal(got, want) {
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

	if got := readTestObject(t, client, key, len(payload)); !bytes.Equal(got, payload) {
		t.Fatalf("object = %q, want %q", got, payload)
	}
}
