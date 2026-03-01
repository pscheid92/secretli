## Why

Users can share encrypted text secrets (Phase 3), but have no way to share encrypted files. File sharing is a core feature of zero-knowledge secret sharing services — users need to share documents, images, credentials files, and other binary data with the same zero-knowledge guarantees as text secrets.

## What Changes

- Add MinIO/S3 client for streaming encrypted file storage (`internal/storage/`)
- Add file upload handler (`POST /api/v1/secrets/file`) — multipart/form-data, streams encrypted blob directly to S3 without buffering the full file in memory, 100MB max via `http.MaxBytesReader`
- Add file download handler (`POST /api/v1/secrets/{publicID}/file`) — streams encrypted blob from S3 to the client, returns `X-Encrypted-Filename` header
- Extend the existing secret repository to persist and query file-related columns (`storage_key`, `encrypted_filename`, `encrypted_size`)
- Extend `DeleteSecret` handler and expired secret cleanup to also remove S3 objects
- Add `encryptFile`/`decryptFile` and filename encryption to the frontend `KeySet` class
- Build `FileUpload` drag-and-drop component and implement `FilePage`
- Extend `RetrievePage` to handle `secret_type: "file"` — download encrypted blob, decrypt client-side, and trigger browser file save

## Capabilities

### New Capabilities
- `s3-storage`: MinIO/S3 client with streaming put/get/delete operations for encrypted file blobs
- `file-handlers`: File upload (multipart streaming to S3) and download (S3 streaming to client) HTTP handlers
- `file-encryption-ui`: FilePage, FileUpload component, file encrypt/decrypt in KeySet, file download in RetrievePage

### Modified Capabilities
- `secret-crud`: Extend repository queries to include file columns; extend delete handler to clean up S3 objects
- `encryption`: Add binary data encrypt/decrypt (`encryptFile`/`decryptFile`) and filename encryption to KeySet
- `api-client`: Add `uploadFile()` and `downloadFile()` API functions
- `secret-sharing-ui`: Extend RetrievePage to handle file-type secrets (download, decrypt, browser save)

## Impact

- **New Go files**: `internal/storage/s3.go`, `internal/handler/file.go`
- **Modified Go files**: `internal/server/server.go`, `internal/server/routes.go`, `internal/store/secret_repo_pg.go`, `internal/model/secret.go`, `internal/handler/secret.go`
- **New frontend files**: `web/frontend/src/components/FileUpload.tsx`
- **Modified frontend files**: `lib/encryption.ts`, `lib/api.ts`, `pages/FilePage.tsx`, `pages/RetrievePage.tsx`
- **New Go dependency**: `github.com/minio/minio-go/v7`
- **No database migrations**: The secrets table already has `storage_key`, `encrypted_filename`, `encrypted_size`, `secret_type` columns
- **No new npm dependencies**: File reading uses the browser File API; encryption uses existing Web Crypto API
