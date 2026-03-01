## 1. S3 Storage Client

- [x] 1.1 Add `github.com/minio/minio-go/v7` dependency (`go get github.com/minio/minio-go/v7`)
- [x] 1.2 Create `internal/storage/s3.go` with `S3Client` struct, `FileStore` interface (`Put`, `Get`, `Delete`), and `NewS3Client` constructor that connects to MinIO and verifies bucket exists
- [x] 1.3 Wire S3 client creation into `internal/server/server.go`: construct client from config fields, pass to route registration

## 2. Repository Updates

- [x] 2.1 Update `internal/store/secret_repo_pg.go` `Create` INSERT to include `storage_key`, `encrypted_filename`, `encrypted_size` columns
- [x] 2.2 Update `GetByPublicID` and `GetAndDeleteByPublicID` SELECT queries to scan `storage_key`, `encrypted_filename`, `encrypted_size` into the `Secret` model
- [x] 2.3 Update `DeleteExpired` to return storage keys of deleted file-type secrets (change return type to include `[]string` of storage keys)

## 3. File Upload Handler

- [x] 3.1 Create `internal/handler/file.go` with `FileHandler` struct holding `SecretRepo` and `FileStore` interfaces, plus `MaxFileSize int64`
- [x] 3.2 Implement `UploadFile` handler: parse multipart form, read `metadata` JSON part, validate required fields and expiration, wrap request body with `http.MaxBytesReader`, stream `file` part to S3 via `FileStore.Put`, hash tokens, create DB record with `secret_type: "file"`, return 201 with `expires_at`
- [x] 3.3 Implement `DownloadFile` handler: extract `publicID` from path, verify `X-Retrieval-Token`, check `secret_type = "file"`, stream S3 object to response with `Content-Type: application/octet-stream` and `X-Encrypted-Filename` header, handle burn-after-read (stream first, then delete DB record + S3 object)

## 4. Existing Handler Updates

- [x] 4.1 Update `DeleteSecret` in `internal/handler/secret.go` to accept a `FileStore` and delete S3 object when `secret_type = "file"` after DB deletion
- [x] 4.2 Update `internal/server/routes.go` to register `POST /api/v1/secrets/file` and `POST /api/v1/secrets/{publicID}/file` routes, pass `FileStore` to handlers

## 5. Frontend File Encryption

- [x] 5.1 Add `encryptFile(data: Uint8Array)` to `KeySet` in `lib/encryption.ts` — AES-256-GCM encrypt raw bytes, return `{ nonce: string, encryptedBlob: Blob }` (nonce as base64url, blob as raw binary)
- [x] 5.2 Add `decryptFile(nonce: string, encryptedBlob: Blob)` to `KeySet` — decrypt and return `Uint8Array`
- [x] 5.3 Add `encryptFilename(filename: string)` to `KeySet` — encrypt filename, return `nonce:ciphertext` format string
- [x] 5.4 Add `decryptFilename(encrypted: string)` to `KeySet` — split on `:`, decode, decrypt, return original filename

## 6. Frontend API Client

- [x] 6.1 Add `uploadFile(metadata, encryptedBlob)` to `lib/api.ts` — send `POST /api/v1/secrets/file` as `multipart/form-data` with `metadata` JSON part and `file` blob part, return `{ expires_at }`
- [x] 6.2 Add `downloadFile(publicID, retrievalToken)` to `lib/api.ts` — send `POST /api/v1/secrets/{publicID}/file` with `X-Retrieval-Token` header, return `{ blob, encryptedFilename, burnAfterRead, passwordProtected }`

## 7. Frontend Components and Pages

- [x] 7.1 Create `web/frontend/src/components/FileUpload.tsx` — drag-and-drop zone + file input, show filename/size, reject files >100MB, `onSelect` callback
- [x] 7.2 Implement `FilePage.tsx` — FileUpload + ExpirationPicker + burn-after-read toggle + optional password + submit button; on submit: generate KeySet, encrypt file + filename, call `uploadFile()`, show SecretResult with share link
- [x] 7.3 Extend `RetrievePage.tsx` to handle `secret_type: "file"` — after text retrieval returns file type, call `downloadFile()`, decrypt file + filename, trigger browser download via `URL.createObjectURL` + anchor click

## 8. Tests

- [x] 8.1 Write handler unit tests for `UploadFile` using mock `FileStore` and `SecretRepo` (success, missing metadata, missing file, file too large, duplicate public_id)
- [x] 8.2 Write handler unit tests for `DownloadFile` (success, invalid token, not found, wrong secret type, burn-after-read)
- [x] 8.3 Write handler unit tests for `DeleteSecret` with S3 cleanup (file secret deletes S3 object, text secret skips S3)

## 9. Verification

- [x] 9.1 Build Go backend (`go build .`) and verify no compilation errors
- [x] 9.2 Build frontend (`npm run build` in `web/frontend/`) and verify no TypeScript errors
- [x] 9.3 Run `go test ./...` and verify all tests pass
- [ ] 9.4 Manual test: start servers + MinIO, upload a file via FilePage, copy share link, open in new tab, verify file downloads and matches original
- [ ] 9.5 Manual test: upload a password-protected file, verify password prompt appears on retrieval and file decrypts correctly
