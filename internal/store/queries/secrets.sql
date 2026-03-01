-- name: CreateSecret :one
INSERT INTO secrets (
    public_id, retrieval_token_hash, deletion_token_hash,
    encrypted_data, nonce, secret_type,
    storage_key, encrypted_filename, encrypted_size,
    burn_after_read, password_protected, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: GetSecretByPublicID :one
SELECT id, public_id, retrieval_token_hash, deletion_token_hash,
    encrypted_data, nonce, secret_type,
    storage_key, encrypted_filename, encrypted_size,
    burn_after_read, password_protected,
    expires_at, created_at, retrieved_at
FROM secrets
WHERE public_id = $1 AND expires_at > NOW();

-- name: GetAndDeleteSecretByPublicID :one
DELETE FROM secrets
WHERE public_id = $1 AND expires_at > NOW()
RETURNING id, public_id, retrieval_token_hash, deletion_token_hash,
    encrypted_data, nonce, secret_type,
    storage_key, encrypted_filename, encrypted_size,
    burn_after_read, password_protected,
    expires_at, created_at, retrieved_at;

-- name: SetSecretRetrievedAt :exec
UPDATE secrets SET retrieved_at = NOW()
WHERE public_id = $1 AND retrieved_at IS NULL;

-- name: DeleteSecret :execrows
DELETE FROM secrets WHERE public_id = $1;

-- name: DeleteExpiredSecrets :many
DELETE FROM secrets WHERE expires_at < NOW()
RETURNING storage_key;
