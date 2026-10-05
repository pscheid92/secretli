-- Short-code transfers: a relay mailbox through which two browsers run a
-- PAKE and hand over an encrypted share link. The server stores only public
-- PAKE shares and ciphertext; the code's words never reach it.
CREATE TABLE transfers
(
    -- Random transfer identity, also the PAKE session id.
    transfer_id         TEXT        PRIMARY KEY,

    -- Public mailbox number typed as the first part of the code.
    nameplate           INTEGER     NOT NULL,

    -- Hashed bearer tokens. Raw tokens are never stored.
    sender_token_hash   TEXT        NOT NULL,
    receiver_token_hash TEXT,

    -- Lifecycle: open until claimed by a receiver, closed by either side.
    state               TEXT        NOT NULL DEFAULT 'open',
    close_reason        TEXT,
    created_at          TIMESTAMPTZ NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    claimed_at          TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ
);

-- A nameplate identifies one active transfer; ended transfers free it.
CREATE UNIQUE INDEX idx_transfers_active_nameplate
    ON transfers (nameplate)
    WHERE state IN ('open', 'claimed');

-- Cleanup lookup.
CREATE INDEX idx_transfers_expires_at
    ON transfers (expires_at);

-- Write-once messages, one per side and phase.
CREATE TABLE transfer_messages
(
    transfer_id TEXT        NOT NULL REFERENCES transfers (transfer_id) ON DELETE CASCADE,
    side        TEXT        NOT NULL,
    phase       TEXT        NOT NULL,
    data        BYTEA       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (transfer_id, side, phase)
);

---- create above / drop below ----
DROP TABLE IF EXISTS transfer_messages;
DROP TABLE IF EXISTS transfers;
