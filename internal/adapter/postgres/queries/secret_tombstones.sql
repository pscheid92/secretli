-- name: CreateTombstone :exec
-- What became of a secret. The first outcome recorded stays: a one-time
-- secret that was opened and then deleted or expired was opened.
INSERT INTO secret_tombstones (
    public_id,
    metadata_token_hash,
    deletion_token_hash,
    outcome,
    burn_after_read,
    ended_at,
    first_opened_at,
    opened_by_owner,
    keep_until
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (public_id) DO NOTHING;

-- name: CreateTombstonesForExpiredSecrets :exec
-- Tombstones for the expired secrets of a cleanup batch. The consumed
-- one-time secrets in the batch have had theirs since they were opened.
INSERT INTO secret_tombstones (
    public_id,
    metadata_token_hash,
    deletion_token_hash,
    outcome,
    burn_after_read,
    ended_at,
    first_opened_at,
    opened_by_owner,
    keep_until
)
SELECT
    public_id,
    metadata_token_hash,
    deletion_token_hash,
    'expired',
    burn_after_read,
    expires_at,
    CASE WHEN burn_after_read THEN NULL ELSE retrieved_at END,
    false,
    sqlc.arg(keep_until)::timestamptz
FROM secrets
WHERE public_id = ANY(sqlc.arg(public_ids)::text[])
  AND expires_at < sqlc.arg(now_at)
ON CONFLICT (public_id) DO NOTHING;

-- name: GetTombstone :one
SELECT *
FROM secret_tombstones
WHERE public_id = sqlc.arg(public_id)
  AND keep_until > sqlc.arg(now_at);

-- name: DeleteExpiredTombstones :execrows
DELETE FROM secret_tombstones
WHERE keep_until <= sqlc.arg(now_at);
