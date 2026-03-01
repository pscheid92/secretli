CREATE TABLE user_secrets (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret_id  BIGINT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
    label      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, secret_id)
);

---- create above / drop below ----
DROP TABLE IF EXISTS user_secrets;
