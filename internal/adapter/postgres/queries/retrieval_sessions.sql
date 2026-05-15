-- name: GetSecretByPublicIDForUpdate :one
SELECT *
FROM secrets
WHERE public_id = sqlc.arg(public_id)
  AND expires_at > sqlc.arg(now_at)
FOR UPDATE;

-- name: CreateRetrievalSession :exec
INSERT INTO retrieval_sessions (
    public_id,
    session_token_hash,
    expires_at,
    created_at
)
VALUES (
    $1, $2, $3, $4
);

-- name: GetSecretByRetrievalSession :one
SELECT
    s.public_id,
    s.metadata_token_hash,
    s.blob_token_hash,
    s.deletion_token_hash,
    s.encrypted_meta,
    s.blob_size,
    s.burn_after_read,
    s.expires_at,
    s.created_at,
    s.retrieved_at
FROM retrieval_sessions AS rs
JOIN secrets AS s ON s.public_id = rs.public_id
WHERE s.public_id = sqlc.arg(public_id)
  AND rs.session_token_hash = sqlc.arg(session_token_hash)
  AND rs.expires_at > sqlc.arg(now_at)
  AND s.expires_at > sqlc.arg(now_at);

-- name: DeleteExpiredRetrievalSessions :execrows
DELETE
FROM retrieval_sessions
WHERE expires_at < sqlc.arg(now_at);
