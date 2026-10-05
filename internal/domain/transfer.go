package domain

import "time"

const (
	TransferStateOpen    = "open"
	TransferStateClaimed = "claimed"
	TransferStateClosed  = "closed"

	TransferSideSender   = "sender"
	TransferSideReceiver = "receiver"

	TransferPhaseShare   = "share"
	TransferPhaseConfirm = "confirm"
	TransferPhasePayload = "payload"

	// Close reasons a side may report; "expired" is set by the server.
	TransferCloseDone      = "done"
	TransferCloseCancelled = "cancelled"
	TransferCloseMismatch  = "mismatch"
	TransferCloseExpired   = "expired"

	// TransferMessageMaxBytes bounds one relayed message. The largest is the
	// sealed payload: a padded 512-byte link plus nonce and tag.
	TransferMessageMaxBytes = 4096
)

// Transfer is a relay mailbox for one short-code hand-over. The server keeps
// only token hashes, public PAKE shares and ciphertext.
type Transfer struct {
	TransferID        string
	Nameplate         int
	SenderTokenHash   string
	ReceiverTokenHash string
	State             string
	CloseReason       string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	ClaimedAt         *time.Time
	ClosedAt          *time.Time
}

// Ended reports whether the transfer was closed or ran out.
func (t *Transfer) Ended(now time.Time) bool {
	return t.State == TransferStateClosed || !now.Before(t.ExpiresAt)
}

type TransferMessage struct {
	TransferID string
	Side       string
	Phase      string
	Data       []byte
	CreatedAt  time.Time
}

func ValidTransferCloseReason(reason string) bool {
	return reason == TransferCloseDone || reason == TransferCloseCancelled || reason == TransferCloseMismatch
}

// OtherTransferSide returns the side whose messages the given side reads.
func OtherTransferSide(side string) string {
	if side == TransferSideSender {
		return TransferSideReceiver
	}
	return TransferSideSender
}
