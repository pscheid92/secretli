-- +goose Up
CREATE TABLE secrets (
    id                   BIGSERIAL PRIMARY KEY,
    public_id            TEXT NOT NULL UNIQUE,
    retrieval_token_hash TEXT NOT NULL,
    deletion_token_hash  TEXT NOT NULL,
    encrypted_data       TEXT,
    nonce                TEXT NOT NULL,
    secret_type          TEXT NOT NULL DEFAULT 'text' CHECK (secret_type IN ('text', 'file')),
    storage_key          TEXT,
    encrypted_filename   TEXT,
    encrypted_size       BIGINT,
    burn_after_read      BOOLEAN NOT NULL DEFAULT FALSE,
    password_protected   BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at           TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retrieved_at         TIMESTAMPTZ
);

CREATE INDEX idx_secrets_public_id ON secrets (public_id);
CREATE INDEX idx_secrets_expires_at ON secrets (expires_at) WHERE retrieved_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS secrets;
