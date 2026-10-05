-- Contract step of migration 007: the relay runs on code_transfers, and no
-- version in production touches these tables any more. Dropping them drops
-- their triggers from migration 006 too; the function notify_transfer_event()
-- stays, code_transfers uses it.
DROP TABLE IF EXISTS transfer_messages;
DROP TABLE IF EXISTS transfers;

---- create above / drop below ----
-- Restores, empty, the tables of migration 005 and the triggers of
-- migration 006.
CREATE TABLE transfers
(
    transfer_id         TEXT        PRIMARY KEY,
    nameplate           INTEGER     NOT NULL,
    sender_token_hash   TEXT        NOT NULL,
    receiver_token_hash TEXT,
    state               TEXT        NOT NULL DEFAULT 'open',
    close_reason        TEXT,
    created_at          TIMESTAMPTZ NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    claimed_at          TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_transfers_active_nameplate
    ON transfers (nameplate)
    WHERE state IN ('open', 'claimed');

CREATE INDEX idx_transfers_expires_at
    ON transfers (expires_at);

CREATE TABLE transfer_messages
(
    transfer_id TEXT        NOT NULL REFERENCES transfers (transfer_id) ON DELETE CASCADE,
    side        TEXT        NOT NULL,
    phase       TEXT        NOT NULL,
    data        BYTEA       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (transfer_id, side, phase)
);

CREATE TRIGGER transfer_messages_notify
    AFTER INSERT
    ON transfer_messages
    FOR EACH ROW
EXECUTE FUNCTION notify_transfer_event();

CREATE TRIGGER transfers_closed_notify
    AFTER UPDATE OF state
    ON transfers
    FOR EACH ROW
    WHEN (NEW.state = 'closed' AND OLD.state IS DISTINCT FROM NEW.state)
EXECUTE FUNCTION notify_transfer_event();
