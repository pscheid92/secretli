-- name: CreateSecret :exec
INSERT INTO secrets (
    public_id, metadata_token_hash, blob_token_hash, deletion_token_hash,
    encrypted_meta, blob_size,
    burn_after_read, expires_at, storage_version, status, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'single-v1', 'active', NOW());

-- name: CreateChunkedUpload :exec
INSERT INTO secrets (
    public_id, metadata_token_hash, blob_token_hash, deletion_token_hash,
    encrypted_meta, blob_size, burn_after_read, expires_at,
    storage_version, status, expiration_duration_seconds,
    upload_token_hash, upload_expires_at, chunk_size, chunk_count,
    encrypted_total_size
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    'chunked-v1', 'pending', $9,
    $10, $11, $12, $13, $14
);

-- name: GetSecretByPublicID :one
SELECT public_id, metadata_token_hash, blob_token_hash, deletion_token_hash,
    encrypted_meta, blob_size,
    burn_after_read,
    expires_at, created_at, retrieved_at,
    storage_version, status, expiration_duration_seconds,
    upload_token_hash, upload_expires_at, chunk_size, chunk_count,
    encrypted_total_size, completed_at
FROM secrets
WHERE public_id = $1 AND status = 'active' AND expires_at > NOW();

-- name: GetPendingUploadByPublicID :one
SELECT public_id, metadata_token_hash, blob_token_hash, deletion_token_hash,
    encrypted_meta, blob_size,
    burn_after_read,
    expires_at, created_at, retrieved_at,
    storage_version, status, expiration_duration_seconds,
    upload_token_hash, upload_expires_at, chunk_size, chunk_count,
    encrypted_total_size, completed_at
FROM secrets
WHERE public_id = $1
  AND status = 'pending'
  AND storage_version = 'chunked-v1'
  AND upload_expires_at > NOW();

-- name: ClaimBurnAfterRead :execrows
UPDATE secrets SET retrieved_at = NOW()
WHERE public_id = $1
  AND blob_token_hash = $2
  AND burn_after_read = true
  AND retrieved_at IS NULL
  AND status = 'active'
  AND expires_at > NOW();

-- name: CreateSecretObject :exec
INSERT INTO secret_objects (
    public_id, object_kind, object_index, encrypted_size, sha256_hex
) VALUES ($1, $2, $3, $4, $5);

-- name: GetSecretObject :one
SELECT public_id, object_kind, object_index, encrypted_size, sha256_hex,
    created_at, updated_at
FROM secret_objects
WHERE public_id = $1 AND object_kind = $2 AND object_index = $3;

-- name: ListSecretObjects :many
SELECT public_id, object_kind, object_index, encrypted_size, sha256_hex,
    created_at, updated_at
FROM secret_objects
WHERE public_id = $1
ORDER BY object_kind, object_index;

-- name: CompleteChunkedUpload :execrows
UPDATE secrets
SET status = 'active',
    expires_at = $2,
    blob_size = $3,
    completed_at = NOW(),
    upload_token_hash = NULL
WHERE public_id = $1
  AND status = 'pending'
  AND storage_version = 'chunked-v1'
  AND upload_expires_at > NOW();

-- name: DeleteSecret :execrows
DELETE FROM secrets WHERE public_id = $1;

-- name: SelectExpiredForCleanup :many
SELECT public_id FROM secrets
WHERE (status = 'active' AND expires_at < NOW())
   OR (
       status = 'active'
       AND
       burn_after_read = true
       AND retrieved_at IS NOT NULL
       AND NOT EXISTS (
           SELECT 1 FROM retrieval_sessions
           WHERE retrieval_sessions.public_id = secrets.public_id
             AND retrieval_sessions.expires_at > NOW()
       )
   )
   OR (
       status = 'pending'
       AND upload_expires_at < NOW()
   )
FOR UPDATE SKIP LOCKED;
