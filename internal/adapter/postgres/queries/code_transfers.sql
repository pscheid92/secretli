-- name: LockTransferNameplates :exec
-- Serializes nameplate allocation across replicas for one transaction, so
-- concurrent creates take consecutive numbers instead of racing for one.
SELECT pg_advisory_xact_lock(7302101);

-- name: CloseExpiredActiveTransfers :exec
-- Frees the nameplates of transfers that ran out before cleanup got to them.
UPDATE code_transfers
SET close_reason = 'expired',
    closed_at    = sqlc.arg(now_at)
WHERE closed_at IS NULL
  AND expires_at <= sqlc.arg(now_at);

-- name: NextFreeNameplate :one
SELECT n::INTEGER AS nameplate
FROM generate_series(1, sqlc.arg(max_nameplate)::INTEGER) AS n
WHERE NOT EXISTS (
    SELECT 1
    FROM code_transfers t
    WHERE t.nameplate = n
      AND t.closed_at IS NULL
)
ORDER BY n
LIMIT 1;

-- name: CreateTransfer :exec
INSERT INTO code_transfers (
    transfer_id,
    nameplate,
    sender_token_hash,
    offer,
    created_at,
    expires_at
)
VALUES (
    $1, $2, $3, $4, $5, $6
);

-- name: ClaimTransfer :one
UPDATE code_transfers
SET receiver_token_hash = sqlc.arg(receiver_token_hash)
WHERE nameplate = sqlc.arg(nameplate)
  AND receiver_token_hash IS NULL
  AND closed_at IS NULL
  AND expires_at > sqlc.arg(now_at)
RETURNING *;

-- name: ClaimedTransferExists :one
SELECT EXISTS (
    SELECT 1
    FROM code_transfers
    WHERE nameplate = sqlc.arg(nameplate)
      AND receiver_token_hash IS NOT NULL
      AND closed_at IS NULL
      AND expires_at > sqlc.arg(now_at)
);

-- name: GetTransfer :one
SELECT *
FROM code_transfers
WHERE transfer_id = $1;

-- name: AnswerTransfer :execrows
-- Writes the receiver's answer once, while the transfer is active.
UPDATE code_transfers
SET answer_share        = sqlc.arg(answer_share),
    answer_confirmation = sqlc.arg(answer_confirmation)
WHERE transfer_id = sqlc.arg(transfer_id)
  AND answer_share IS NULL
  AND receiver_token_hash IS NOT NULL
  AND closed_at IS NULL
  AND expires_at > sqlc.arg(now_at);

-- name: DeliverTransfer :execrows
-- Writes the sealed link once, after the answer, and closes the transfer.
UPDATE code_transfers
SET delivery     = sqlc.arg(delivery),
    close_reason = 'done',
    closed_at    = sqlc.arg(now_at)
WHERE transfer_id = sqlc.arg(transfer_id)
  AND answer_share IS NOT NULL
  AND closed_at IS NULL
  AND expires_at > sqlc.arg(now_at);

-- name: CloseTransfer :execrows
UPDATE code_transfers
SET close_reason = sqlc.arg(close_reason),
    closed_at    = sqlc.arg(now_at)
WHERE transfer_id = sqlc.arg(transfer_id)
  AND closed_at IS NULL;

-- name: DeleteEndedTransfers :execrows
DELETE FROM code_transfers
WHERE expires_at < sqlc.arg(ended_before)
   OR closed_at < sqlc.arg(ended_before);
