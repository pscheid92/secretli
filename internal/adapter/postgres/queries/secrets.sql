-- name: CreateSecret :exec
INSERT INTO secrets (
    public_id, retrieval_token, deletion_token,
    encrypted_meta, blob_size,
    burn_after_read, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetSecretByPublicID :one
SELECT public_id, retrieval_token, deletion_token,
    encrypted_meta, blob_size,
    burn_after_read,
    expires_at, created_at, retrieved_at
FROM secrets
WHERE public_id = $1 AND expires_at > NOW();

-- name: GetAndDeleteSecretByPublicID :one
DELETE FROM secrets
WHERE public_id = $1 AND expires_at > NOW()
RETURNING public_id, retrieval_token, deletion_token,
    encrypted_meta, blob_size,
    burn_after_read,
    expires_at, created_at, retrieved_at;

-- name: SetSecretRetrievedAt :exec
UPDATE secrets SET retrieved_at = NOW()
WHERE public_id = $1 AND retrieved_at IS NULL;

-- name: DeleteSecret :execrows
DELETE FROM secrets WHERE public_id = $1;

-- name: DeleteExpiredSecrets :many
DELETE FROM secrets WHERE expires_at < NOW()
RETURNING public_id;
