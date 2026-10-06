-- What became of a secret, kept for a while after the secret itself is gone,
-- so a link to it can say whether it was opened, expired unopened or was
-- deleted. A tombstone holds no content and no key material: the public id,
-- the token hashes that guard the answer, the outcome and when it happened.
CREATE TABLE secret_tombstones
(
    public_id           TEXT        PRIMARY KEY,

    -- Hashed bearer tokens, copied from the secret. The metadata token, part
    -- of every link, unlocks the answer; the deletion token is in the owner
    -- link only.
    metadata_token_hash TEXT        NOT NULL,
    deletion_token_hash TEXT        NOT NULL,

    -- opened (a one-time secret was read), expired or deleted. The first
    -- outcome recorded stays.
    outcome             TEXT        NOT NULL,
    burn_after_read     BOOLEAN     NOT NULL,

    -- When it happened; for an expired secret, its expiry.
    ended_at            TIMESTAMPTZ NOT NULL,
    -- When a recipient first opened it, if anyone did. A one-time secret was
    -- opened at ended_at, unless the owner opened it themselves.
    first_opened_at     TIMESTAMPTZ,
    opened_by_owner     BOOLEAN     NOT NULL DEFAULT FALSE,

    -- Cleanup forgets the tombstone after this.
    keep_until          TIMESTAMPTZ NOT NULL,

    CHECK (outcome IN ('opened', 'expired', 'deleted'))
);

CREATE INDEX idx_secret_tombstones_keep_until
    ON secret_tombstones (keep_until);

---- create above / drop below ----
DROP TABLE IF EXISTS secret_tombstones;
