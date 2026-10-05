-- Wakes waiting long-polls on every replica: a stored transfer message or a
-- closed transfer sends its transfer_id on the transfer_events channel.
-- Postgres delivers a notification only once the writing transaction has
-- committed, so a woken reader always finds the row.
CREATE FUNCTION notify_transfer_event() RETURNS trigger
    LANGUAGE plpgsql AS
$$
BEGIN
    PERFORM pg_notify('transfer_events', NEW.transfer_id);
    RETURN NULL;
END;
$$;

CREATE TRIGGER transfer_messages_notify
    AFTER INSERT
    ON transfer_messages
    FOR EACH ROW
EXECUTE FUNCTION notify_transfer_event();

-- Closing by either side, and closing on expiry when a new transfer frees
-- nameplates. Claims don't wake anyone: no long-poll waits for them.
CREATE TRIGGER transfers_closed_notify
    AFTER UPDATE OF state
    ON transfers
    FOR EACH ROW
    WHEN (NEW.state = 'closed' AND OLD.state IS DISTINCT FROM NEW.state)
EXECUTE FUNCTION notify_transfer_event();

---- create above / drop below ----
DROP TRIGGER IF EXISTS transfers_closed_notify ON transfers;
DROP TRIGGER IF EXISTS transfer_messages_notify ON transfer_messages;
DROP FUNCTION IF EXISTS notify_transfer_event();
