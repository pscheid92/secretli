-- Active, retrievable secrets.
CREATE TABLE secrets
(
    -- Public lookup key.
    public_id           TEXT        PRIMARY KEY,

    -- Hashed bearer tokens. Raw tokens are never stored.
    metadata_token_hash TEXT        NOT NULL,
    blob_token_hash     TEXT        NOT NULL,
    deletion_token_hash TEXT        NOT NULL,

    -- Client-encrypted metadata envelope and encrypted object size.
    encrypted_meta      TEXT        NOT NULL,
    blob_size           BIGINT      NOT NULL,

    -- Retrieval behavior and lifecycle.
    burn_after_read     BOOLEAN     NOT NULL DEFAULT FALSE,

    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    retrieved_at        TIMESTAMPTZ
);

-- Cleanup indexes.
CREATE INDEX idx_secrets_expires_at
    ON secrets (expires_at);

CREATE INDEX idx_secrets_consumed_burn_after_read
    ON secrets (expires_at, public_id)
    WHERE burn_after_read = true
      AND retrieved_at IS NOT NULL;

---- create above / drop below ----
DROP TABLE IF EXISTS secrets;
