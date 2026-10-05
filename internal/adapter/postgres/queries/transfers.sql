-- name: LockTransferNameplates :exec
-- Serializes nameplate allocation across replicas for one transaction, so
-- concurrent creates take consecutive numbers instead of racing for one.
SELECT pg_advisory_xact_lock(7302101);

-- name: CloseExpiredActiveTransfers :exec
-- Frees the nameplates of transfers that ran out before cleanup got to them.
UPDATE transfers
SET state        = 'closed',
    close_reason = 'expired',
    closed_at    = sqlc.arg(now_at)
WHERE state IN ('open', 'claimed')
  AND expires_at <= sqlc.arg(now_at);

-- name: NextFreeNameplate :one
SELECT n::INTEGER AS nameplate
FROM generate_series(1, sqlc.arg(max_nameplate)::INTEGER) AS n
WHERE NOT EXISTS (
    SELECT 1
    FROM transfers t
    WHERE t.nameplate = n
      AND t.state IN ('open', 'claimed')
)
ORDER BY n
LIMIT 1;

-- name: CreateTransfer :exec
INSERT INTO transfers (
    transfer_id,
    nameplate,
    sender_token_hash,
    state,
    created_at,
    expires_at
)
VALUES (
    $1, $2, $3, 'open', $4, $5
);

-- name: ClaimTransfer :one
UPDATE transfers
SET state               = 'claimed',
    receiver_token_hash = sqlc.arg(receiver_token_hash),
    claimed_at          = sqlc.arg(now_at)
WHERE nameplate = sqlc.arg(nameplate)
  AND state = 'open'
  AND expires_at > sqlc.arg(now_at)
RETURNING *;

-- name: ClaimedTransferExists :one
SELECT EXISTS (
    SELECT 1
    FROM transfers
    WHERE nameplate = sqlc.arg(nameplate)
      AND state = 'claimed'
      AND expires_at > sqlc.arg(now_at)
);

-- name: GetTransfer :one
SELECT *
FROM transfers
WHERE transfer_id = $1;

-- name: CreateTransferMessage :execrows
INSERT INTO transfer_messages (
    transfer_id,
    side,
    phase,
    data,
    created_at
)
VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (transfer_id, side, phase) DO NOTHING;

-- name: GetTransferMessage :one
SELECT *
FROM transfer_messages
WHERE transfer_id = $1
  AND side = $2
  AND phase = $3;

-- name: CloseTransfer :execrows
UPDATE transfers
SET state        = 'closed',
    close_reason = sqlc.arg(close_reason),
    closed_at    = sqlc.arg(now_at)
WHERE transfer_id = sqlc.arg(transfer_id)
  AND state IN ('open', 'claimed');

-- name: DeleteEndedTransfers :execrows
DELETE FROM transfers
WHERE expires_at < sqlc.arg(ended_before)
   OR closed_at < sqlc.arg(ended_before);
