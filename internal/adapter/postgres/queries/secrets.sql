-- name: CreateSecret :exec
INSERT INTO secrets (
    public_id, metadata_token, blob_token, deletion_token,
    encrypted_meta, blob_size,
    burn_after_read, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetSecretByPublicID :one
SELECT public_id, metadata_token, blob_token, deletion_token,
    encrypted_meta, blob_size,
    burn_after_read,
    expires_at, created_at, retrieved_at
FROM secrets
WHERE public_id = $1 AND expires_at > NOW();

-- name: ClaimBurnAfterRead :execrows
UPDATE secrets SET retrieved_at = NOW()
WHERE public_id = $1
  AND blob_token = $2
  AND burn_after_read = true
  AND retrieved_at IS NULL
  AND expires_at > NOW();

-- name: DeleteSecret :execrows
DELETE FROM secrets WHERE public_id = $1;

-- name: SelectExpiredForCleanup :many
SELECT public_id FROM secrets
WHERE expires_at < NOW()
   OR (burn_after_read = true AND retrieved_at IS NOT NULL)
FOR UPDATE SKIP LOCKED;
