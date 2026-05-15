-- Short-lived authorization for range downloads.
CREATE TABLE retrieval_sessions
(
    -- Internal row identity and the active secret this session can read.
    id                 BIGSERIAL   PRIMARY KEY,
    public_id          TEXT        NOT NULL REFERENCES secrets (public_id) ON DELETE CASCADE,

    -- Hashed session bearer token.
    session_token_hash TEXT        NOT NULL UNIQUE,

    -- Session lifecycle.
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL
);

-- Lookup and cleanup indexes.
CREATE INDEX idx_retrieval_sessions_public_expires_at
    ON retrieval_sessions (public_id, expires_at);

CREATE INDEX idx_retrieval_sessions_expires_at
    ON retrieval_sessions (expires_at);

---- create above / drop below ----
DROP TABLE IF EXISTS retrieval_sessions;
