-- name: CreateSecret :exec
INSERT INTO secrets (
    public_id,
    metadata_token_hash,
    blob_token_hash,
    deletion_token_hash,
    encrypted_meta,
    blob_size,
    burn_after_read,
    expires_at,
    created_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
);

-- name: GetSecretByPublicID :one
SELECT
    public_id,
    metadata_token_hash,
    blob_token_hash,
    deletion_token_hash,
    encrypted_meta,
    blob_size,
    burn_after_read,
    expires_at,
    created_at,
    retrieved_at
FROM secrets
WHERE public_id = sqlc.arg(public_id)
  AND expires_at > sqlc.arg(now_at);

-- name: ClaimBurnAfterRead :execrows
UPDATE secrets
SET retrieved_at = sqlc.arg(now_at)
WHERE public_id = sqlc.arg(public_id)
  AND blob_token_hash = sqlc.arg(blob_token_hash)
  AND burn_after_read = true
  AND retrieved_at IS NULL
  AND expires_at > sqlc.arg(now_at);

-- name: DeleteSecret :execrows
DELETE FROM secrets
WHERE public_id = $1;

-- name: SelectExpiredSecretsForCleanup :many
SELECT s.public_id
FROM secrets AS s
WHERE s.expires_at < sqlc.arg(now_at)
FOR UPDATE OF s SKIP LOCKED;

-- name: SelectConsumedBurnAfterReadSecretsForCleanup :many
SELECT s.public_id
FROM secrets AS s
WHERE s.burn_after_read = true
  AND s.retrieved_at IS NOT NULL
  AND s.expires_at >= sqlc.arg(now_at)
  AND NOT EXISTS (
      SELECT 1
      FROM retrieval_sessions AS rs
      WHERE rs.public_id = s.public_id
        AND rs.expires_at > sqlc.arg(now_at)
  )
FOR UPDATE OF s SKIP LOCKED;
