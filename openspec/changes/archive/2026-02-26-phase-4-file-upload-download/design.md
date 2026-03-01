## Context

Phase 3 delivered the frontend encryption protocol and text secret sharing UI. The Go backend has working CRUD endpoints for text secrets stored in PostgreSQL. The database schema already includes file-specific columns (`secret_type`, `storage_key`, `encrypted_filename`, `encrypted_size`) but they are unused. Docker Compose already provisions MinIO with a `secretli` bucket. The S3 config fields are already present in `internal/config/config.go`. The `minio-go/v7` dependency has not been added yet.

The frontend `KeySet` class supports text encryption (`encrypt`/`decrypt` operating on UTF-8 strings) but not binary data. `FilePage.tsx` is a placeholder stub.

## Goals / Non-Goals

**Goals:**
- Streaming file upload to S3 — the server MUST NOT buffer the full encrypted file in memory
- Streaming file download from S3 — stream encrypted blob directly to the HTTP response
- Client-side file encryption/decryption using the same AES-256-GCM protocol as text secrets
- Client-side filename encryption before sending to the server
- File size limit of 100MB enforced server-side via `http.MaxBytesReader`
- Full round-trip: select file → encrypt in browser → upload → share link → download → decrypt → browser save
- Burn-after-read and password protection work the same as text secrets
- S3 object cleanup on secret deletion and expiration

**Non-Goals:**
- Chunked/resumable uploads (single request upload is sufficient for 100MB)
- Progress bars for encryption (Phase 8 polish)
- User authentication or linking secrets to users (Phase 5)
- Multiple file upload in a single secret
- File preview in browser (just download the decrypted file)

## Decisions

### 1. In-browser encryption before upload
**Choice:** Read the entire file into memory in the browser, encrypt with AES-256-GCM, then upload the encrypted blob as a multipart form part.
**Why:** Matches the zero-knowledge protocol from DESIGN.md. The 100MB limit is within browser memory constraints. Streaming encryption would require a chunked protocol that adds significant complexity for minimal benefit at this file size.
**Alternative:** Stream encryption with ReadableStream — rejected as over-engineering for 100MB files.

### 2. Multipart upload with metadata JSON part
**Choice:** `POST /api/v1/secrets/file` accepts `multipart/form-data` with a `metadata` JSON part (containing public_id, tokens, nonce, expiration, etc.) and a `file` binary part (the encrypted blob).
**Why:** Separating metadata from the file blob allows the server to parse metadata first, validate it, then stream the file part directly to S3 without buffering. This is the pattern from DESIGN.md section 5.

### 3. S3 object key format
**Choice:** Use `secrets/{public_id}` as the S3 object key.
**Why:** Simple, unique (public_id is unique in the database), and easy to correlate with the database record for cleanup.

### 4. File download via streaming proxy
**Choice:** `POST /api/v1/secrets/{publicID}/file` streams the S3 object through the Go server to the client. The encrypted filename is returned in an `X-Encrypted-Filename` response header.
**Why:** Avoids presigned URLs which would expose the S3 endpoint and bucket structure. The Go server acts as a proxy, maintaining the zero-knowledge boundary. Uses POST (not GET) for consistency with text retrieval and because it has side effects (burn-after-read, `retrieved_at`).

### 5. Frontend file encryption as raw bytes
**Choice:** Add `encryptFile(data: Uint8Array)` returning `{ nonce: string, encryptedBlob: Blob }` and `decryptFile(nonce: string, encryptedBlob: Blob)` returning `Uint8Array` to the `KeySet` class. The nonce is base64url-encoded; the encrypted blob stays as binary (not base64-encoded) to avoid 33% size inflation.
**Why:** For files, base64 encoding the entire encrypted blob would increase size by ~33% and require the full encoded string in memory. Keeping it as binary (Blob/Uint8Array) is more efficient for multipart upload and download. The nonce is small enough to base64-encode for the metadata JSON.

### 6. Filename encryption
**Choice:** Encrypt the filename as a UTF-8 string using the existing `KeySet.encrypt()` method (returns base64url nonce + ciphertext). Store the combined `nonce:ciphertext` as `encrypted_filename` in the database. On retrieval, the server returns it in the `X-Encrypted-Filename` header for the client to decrypt.
**Why:** Reuses the existing text encryption method. The filename is small, so base64 overhead is negligible.

### 7. Repository changes — add file columns to existing queries
**Choice:** Update the existing `Create`, `GetByPublicID`, and `GetAndDeleteByPublicID` methods in `secret_repo_pg.go` to include `storage_key`, `encrypted_filename`, and `encrypted_size` columns. These are nullable and will be NULL for text secrets.
**Why:** No new repository interface methods needed — the existing `Secret` model already has these fields. Just need to include them in SQL queries.

### 8. S3 cleanup on delete and expiry
**Choice:** Extend `DeleteSecret` handler to call `s3.Delete(storageKey)` after DB deletion when `secret_type = "file"`. Modify `DeleteExpired` repo method to return storage keys of deleted file secrets so the caller can clean them up.
**Why:** Keeps S3 in sync with the database. Orphaned S3 objects would waste storage.

## Risks / Trade-offs

- **[100MB in browser memory]** → Modern browsers handle this fine. The `File.arrayBuffer()` API is well-supported. Users on low-memory devices may struggle with very large files, but 100MB is a reasonable limit.
- **[Server streaming without full buffering]** → Uses `io.Copy` between multipart reader and S3 `PutObject`. If S3 is slow or the upload is interrupted, the partial object may be orphaned. Mitigation: the cleanup worker will eventually remove expired secrets and their S3 objects.
- **[No upload progress indicator]** → The browser's native upload progress isn't easily accessible with `fetch`. Mitigation: show a loading spinner; add proper progress in Phase 8.
- **[WriteTimeout for large uploads]** → The default 60s WriteTimeout may not be enough for slow connections uploading 100MB. Mitigation: use `http.ResponseController` to extend the deadline per-request in the file upload handler.
