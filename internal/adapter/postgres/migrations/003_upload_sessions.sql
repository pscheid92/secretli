-- Pending backend-owned S3 multipart uploads.
CREATE TABLE upload_sessions
(
    -- Upload-session identity and final public share ID.
    session_id          TEXT        PRIMARY KEY,
    public_id           TEXT        NOT NULL,

    -- Hashed bearer tokens. Raw tokens are never stored.
    upload_token_hash   TEXT        NOT NULL,
    metadata_token_hash TEXT        NOT NULL,
    blob_token_hash     TEXT        NOT NULL,
    deletion_token_hash TEXT        NOT NULL,

    -- Provider multipart-upload handle.
    s3_upload_id        TEXT        NOT NULL,

    -- Expected final encrypted object size.
    blob_size           BIGINT      NOT NULL,

    -- Secret fields copied into secrets only after multipart completion.
    encrypted_meta      TEXT        NOT NULL,
    burn_after_read     BOOLEAN     NOT NULL DEFAULT FALSE,
    secret_expires_at   TIMESTAMPTZ NOT NULL,

    -- Upload-session lifecycle.
    upload_expires_at   TIMESTAMPTZ NOT NULL,
    state               TEXT        NOT NULL DEFAULT 'pending',
    created_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ,
    aborted_at          TIMESTAMPTZ
);

-- A public ID can have one active pending upload session.
CREATE UNIQUE INDEX idx_upload_sessions_pending_public_id
    ON upload_sessions (public_id)
    WHERE state = 'pending';

-- Cleanup lookup for expired pending sessions.
CREATE INDEX idx_upload_sessions_expired_pending
    ON upload_sessions (upload_expires_at)
    WHERE state = 'pending';

-- Uploaded S3 multipart parts recorded for idempotency and completion.
CREATE TABLE upload_parts
(
    -- Parent upload session and S3 part number.
    session_id  TEXT        NOT NULL REFERENCES upload_sessions (session_id) ON DELETE CASCADE,
    part_number INTEGER     NOT NULL,

    -- Byte placement and integrity for the final encrypted object.
    part_offset BIGINT      NOT NULL,
    part_size   BIGINT      NOT NULL,
    part_sha256 TEXT        NOT NULL,

    -- S3 completion material.
    etag        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (session_id, part_number)
);

---- create above / drop below ----
DROP TABLE IF EXISTS upload_parts;
DROP TABLE IF EXISTS upload_sessions;
