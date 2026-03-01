## ADDED Requirements

### Requirement: S3 client initialization
The `internal/storage/s3.go` module SHALL provide a `NewS3Client(endpoint, bucket, accessKey, secretKey, region string, useSSL bool)` constructor that returns an S3 client connected to the configured MinIO/S3 endpoint. The client SHALL verify bucket existence on initialization.

#### Scenario: Successful client creation
- **WHEN** `NewS3Client` is called with valid MinIO credentials
- **THEN** the client connects to the endpoint and verifies the bucket exists

#### Scenario: Bucket does not exist
- **WHEN** the configured bucket does not exist on the S3 endpoint
- **THEN** initialization returns an error

### Requirement: Streaming file upload to S3
The S3 client SHALL provide a `Put(ctx, key string, reader io.Reader, size int64) error` method that streams data from the reader directly to the S3 object without buffering the full content in memory.

#### Scenario: Upload a file
- **WHEN** `Put("secrets/abc123", reader, 1024)` is called
- **THEN** the data from the reader is stored as an S3 object at key `secrets/abc123`

#### Scenario: Upload with unknown size
- **WHEN** `Put` is called with `size = -1`
- **THEN** the upload succeeds using chunked/multipart S3 upload

### Requirement: Streaming file download from S3
The S3 client SHALL provide a `Get(ctx, key string) (io.ReadCloser, error)` method that returns a reader for streaming the S3 object content without buffering it fully in memory.

#### Scenario: Download a file
- **WHEN** `Get("secrets/abc123")` is called for an existing object
- **THEN** an `io.ReadCloser` is returned that streams the object content

#### Scenario: Object not found
- **WHEN** `Get` is called for a non-existent key
- **THEN** an error is returned

### Requirement: Delete S3 object
The S3 client SHALL provide a `Delete(ctx, key string) error` method that removes an object from S3.

#### Scenario: Delete an existing object
- **WHEN** `Delete("secrets/abc123")` is called
- **THEN** the S3 object is removed

#### Scenario: Delete a non-existent object
- **WHEN** `Delete` is called for a key that does not exist
- **THEN** the method succeeds without error (idempotent)
