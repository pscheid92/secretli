-- Short-code transfers as one row each. The protocol has exactly three
-- legs, so their messages are columns instead of rows of a second table:
-- the sender's offer, the receiver's answer and the sender's delivery.
-- The server stores public PAKE shares, a confirmation tag and ciphertext;
-- the code's words never reach it.
--
-- The tables of migration 005 stay until a later migration, so the previous
-- version keeps working while a deploy rolls out.
CREATE TABLE code_transfers
(
    -- 32 random bytes, base64url, chosen by the sender: it is also the PAKE
    -- session id, and the offer depends on it.
    transfer_id         TEXT        PRIMARY KEY,

    -- The number at the start of the code. Unique among active transfers.
    nameplate           INTEGER     NOT NULL,

    -- Hashed bearer tokens. Raw tokens are never stored.
    sender_token_hash   TEXT        NOT NULL,
    receiver_token_hash TEXT,

    -- The sender's PAKE share, stored when the transfer is created.
    offer               BYTEA       NOT NULL,

    -- The receiver's PAKE share and confirmation tag, written together once.
    answer_share        BYTEA,
    answer_confirmation BYTEA,

    -- The sealed link. Storing it closes the transfer as done.
    delivery            BYTEA,

    -- done, cancelled, mismatch or expired, set together with closed_at.
    close_reason        TEXT,

    created_at          TIMESTAMPTZ NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    closed_at           TIMESTAMPTZ,

    CHECK ((answer_share IS NULL) = (answer_confirmation IS NULL)),
    CHECK ((close_reason IS NULL) = (closed_at IS NULL))
);

-- A nameplate names at most one active transfer; ended ones free it.
CREATE UNIQUE INDEX idx_code_transfers_active_nameplate
    ON code_transfers (nameplate)
    WHERE closed_at IS NULL;

CREATE INDEX idx_code_transfers_expires_at
    ON code_transfers (expires_at);

-- Every change of a transfer (claim, answer, delivery, close) wakes the
-- long-polls waiting on it, on every replica.
CREATE TRIGGER code_transfers_notify
    AFTER UPDATE
    ON code_transfers
    FOR EACH ROW
EXECUTE FUNCTION notify_transfer_event();

---- create above / drop below ----
DROP TABLE IF EXISTS code_transfers;
