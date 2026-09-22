-- name: SecretExistsByPublicID :one
SELECT EXISTS (
    SELECT 1
    FROM secrets
    WHERE public_id = $1
);

-- name: CreateUploadSession :exec
INSERT INTO upload_sessions (
    session_id,
    public_id,
    upload_token_hash,
    metadata_token_hash,
    blob_token_hash,
    deletion_token_hash,
    s3_upload_id,
    blob_size,
    encrypted_meta,
    burn_after_read,
    secret_expires_at,
    upload_expires_at,
    created_at,
    storage_key,
    state
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'pending'
);

-- name: GetUploadSession :one
SELECT *
FROM upload_sessions
WHERE session_id = $1;

-- name: GetUploadSessionForUpdate :one
SELECT *
FROM upload_sessions
WHERE session_id = $1
FOR UPDATE;

-- name: UploadSessionExists :one
SELECT EXISTS (
    SELECT 1
    FROM upload_sessions
    WHERE session_id = $1
);

-- name: ListExpiredUploadSessionsForUpdate :many
SELECT *
FROM upload_sessions
WHERE state = 'pending'
  AND upload_expires_at < sqlc.arg(now_at)
FOR UPDATE SKIP LOCKED;

-- name: MarkUploadSessionCompleted :exec
-- The share material now lives on the secret row; the session keeps only
-- what a repeated complete needs.
UPDATE upload_sessions
SET state = 'completed',
    completed_at = sqlc.arg(now_at),
    metadata_token_hash = NULL,
    blob_token_hash = NULL,
    deletion_token_hash = NULL,
    encrypted_meta = NULL
WHERE session_id = sqlc.arg(session_id);

-- name: MarkUploadSessionAborted :execrows
UPDATE upload_sessions
SET state = 'aborted',
    aborted_at = sqlc.arg(now_at),
    metadata_token_hash = NULL,
    blob_token_hash = NULL,
    deletion_token_hash = NULL,
    encrypted_meta = NULL
WHERE session_id = sqlc.arg(session_id)
  AND state = 'pending';

-- name: ListUploadPartsBySession :many
SELECT *
FROM upload_parts
WHERE session_id = $1
ORDER BY part_number;

-- name: GetUploadPartForUpdate :one
SELECT *
FROM upload_parts
WHERE session_id = $1
  AND part_number = $2
FOR UPDATE;

-- name: CreateUploadPart :one
INSERT INTO upload_parts (
    session_id,
    part_number,
    part_offset,
    part_size,
    part_sha256,
    etag,
    created_at
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: DeleteUploadPartsBySession :exec
DELETE FROM upload_parts
WHERE session_id = $1;

-- name: DeleteFinishedUploadSessions :execrows
DELETE FROM upload_sessions
WHERE state <> 'pending'
  AND COALESCE(completed_at, aborted_at) < sqlc.arg(finished_before);
