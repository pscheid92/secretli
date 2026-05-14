ALTER TABLE secrets
    ADD COLUMN storage_version TEXT NOT NULL DEFAULT 'single-v1',
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN expiration_duration_seconds BIGINT,
    ADD COLUMN upload_token_hash TEXT,
    ADD COLUMN upload_expires_at TIMESTAMPTZ,
    ADD COLUMN chunk_size BIGINT,
    ADD COLUMN chunk_count INTEGER,
    ADD COLUMN encrypted_total_size BIGINT,
    ADD COLUMN completed_at TIMESTAMPTZ;

UPDATE secrets
SET completed_at = created_at
WHERE completed_at IS NULL;

ALTER TABLE secrets
    ADD CONSTRAINT secrets_storage_version_valid
        CHECK (storage_version IN ('single-v1', 'chunked-v1')),
    ADD CONSTRAINT secrets_status_valid
        CHECK (status IN ('pending', 'active')),
    ADD CONSTRAINT secrets_upload_token_hash_length
        CHECK (upload_token_hash IS NULL OR length(upload_token_hash) = 64),
    ADD CONSTRAINT secrets_chunked_upload_shape
        CHECK (
            storage_version = 'single-v1'
            OR (
                chunk_size IS NOT NULL
                AND chunk_size > 0
                AND chunk_count IS NOT NULL
                AND chunk_count >= 0
                AND encrypted_total_size IS NOT NULL
                AND encrypted_total_size > 0
            )
        ),
    ADD CONSTRAINT secrets_pending_upload_shape
        CHECK (
            status = 'active'
            OR (
                storage_version = 'chunked-v1'
                AND upload_token_hash IS NOT NULL
                AND upload_expires_at IS NOT NULL
                AND expiration_duration_seconds IS NOT NULL
                AND expiration_duration_seconds > 0
            )
        );

CREATE INDEX idx_secrets_pending_upload_expires_at
    ON secrets (upload_expires_at)
    WHERE status = 'pending';

CREATE TABLE secret_objects (
    public_id      TEXT NOT NULL REFERENCES secrets(public_id) ON DELETE CASCADE,
    object_kind    TEXT NOT NULL,
    object_index   INTEGER NOT NULL,
    encrypted_size BIGINT NOT NULL,
    sha256_hex     TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (public_id, object_kind, object_index),
    CONSTRAINT secret_objects_kind_valid
        CHECK (object_kind IN ('manifest', 'chunk')),
    CONSTRAINT secret_objects_manifest_index
        CHECK (
            (object_kind = 'manifest' AND object_index = -1)
            OR (object_kind = 'chunk' AND object_index >= 0)
        ),
    CONSTRAINT secret_objects_encrypted_size_positive
        CHECK (encrypted_size > 0),
    CONSTRAINT secret_objects_sha256_hex_length
        CHECK (length(sha256_hex) = 64)
);

CREATE INDEX idx_secret_objects_public_id
    ON secret_objects (public_id);

---- create above / drop below ----
DROP TABLE IF EXISTS secret_objects;

DROP INDEX IF EXISTS idx_secrets_pending_upload_expires_at;

ALTER TABLE secrets
    DROP CONSTRAINT IF EXISTS secrets_pending_upload_shape,
    DROP CONSTRAINT IF EXISTS secrets_chunked_upload_shape,
    DROP CONSTRAINT IF EXISTS secrets_upload_token_hash_length,
    DROP CONSTRAINT IF EXISTS secrets_status_valid,
    DROP CONSTRAINT IF EXISTS secrets_storage_version_valid,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS encrypted_total_size,
    DROP COLUMN IF EXISTS chunk_count,
    DROP COLUMN IF EXISTS chunk_size,
    DROP COLUMN IF EXISTS upload_expires_at,
    DROP COLUMN IF EXISTS upload_token_hash,
    DROP COLUMN IF EXISTS expiration_duration_seconds,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS storage_version;
