CREATE TABLE secrets (
    public_id          TEXT PRIMARY KEY,
    retrieval_token    TEXT NOT NULL,
    deletion_token     TEXT NOT NULL,
    encrypted_meta     TEXT NOT NULL,
    blob_size          BIGINT NOT NULL,
    burn_after_read    BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retrieved_at       TIMESTAMPTZ
);

CREATE INDEX idx_secrets_expires_at ON secrets (expires_at) WHERE retrieved_at IS NULL;

---- create above / drop below ----
DROP TABLE IF EXISTS secrets;
