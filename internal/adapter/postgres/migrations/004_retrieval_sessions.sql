CREATE TABLE retrieval_sessions (
    id                 BIGSERIAL PRIMARY KEY,
    public_id          TEXT NOT NULL REFERENCES secrets(public_id) ON DELETE CASCADE,
    session_token_hash TEXT NOT NULL UNIQUE,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_retrieval_sessions_public_id ON retrieval_sessions (public_id);
CREATE INDEX idx_retrieval_sessions_expires_at ON retrieval_sessions (expires_at);

---- create above / drop below ----
DROP TABLE IF EXISTS retrieval_sessions;
