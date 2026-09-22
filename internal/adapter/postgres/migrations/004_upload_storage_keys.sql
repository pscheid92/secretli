-- Every upload writes to its own object key, recorded on the session and
-- copied to the secret, so an upload can only ever overwrite or delete its own
-- object. Existing rows keep the legacy key derived from the public ID.
ALTER TABLE secrets ADD COLUMN storage_key TEXT;
UPDATE secrets SET storage_key = 'secrets/' || public_id;
ALTER TABLE secrets ALTER COLUMN storage_key SET NOT NULL;

ALTER TABLE upload_sessions ADD COLUMN storage_key TEXT;
UPDATE upload_sessions SET storage_key = 'secrets/' || public_id;
ALTER TABLE upload_sessions ALTER COLUMN storage_key SET NOT NULL;

-- Only pending sessions hold share material. A finished session is kept as a
-- tombstone (enough to answer a repeated complete or abort) and purged by
-- cleanup, so nothing about a share outlives its secret row.
ALTER TABLE upload_sessions
    ALTER COLUMN metadata_token_hash DROP NOT NULL,
    ALTER COLUMN blob_token_hash DROP NOT NULL,
    ALTER COLUMN deletion_token_hash DROP NOT NULL,
    ALTER COLUMN encrypted_meta DROP NOT NULL;

UPDATE upload_sessions
SET metadata_token_hash = NULL,
    blob_token_hash     = NULL,
    deletion_token_hash = NULL,
    encrypted_meta      = NULL
WHERE state <> 'pending';

DELETE FROM upload_parts AS p
USING upload_sessions AS s
WHERE s.session_id = p.session_id
  AND s.state <> 'pending';

-- Cleanup lookup for finished sessions.
CREATE INDEX idx_upload_sessions_finished
    ON upload_sessions ((COALESCE(completed_at, aborted_at)))
    WHERE state <> 'pending';

---- create above / drop below ----
-- Objects written under per-upload keys become unreachable after this.
DROP INDEX IF EXISTS idx_upload_sessions_finished;
DELETE FROM upload_sessions WHERE state <> 'pending';
ALTER TABLE upload_sessions
    ALTER COLUMN metadata_token_hash SET NOT NULL,
    ALTER COLUMN blob_token_hash SET NOT NULL,
    ALTER COLUMN deletion_token_hash SET NOT NULL,
    ALTER COLUMN encrypted_meta SET NOT NULL;
ALTER TABLE upload_sessions DROP COLUMN storage_key;
ALTER TABLE secrets DROP COLUMN storage_key;
