package domain

import "time"

const (
	TransferSideSender   = "sender"
	TransferSideReceiver = "receiver"

	// Close reasons. The delivery closes a transfer as done and the server
	// marks it expired; a side may report cancelled or mismatch.
	TransferCloseDone      = "done"
	TransferCloseCancelled = "cancelled"
	TransferCloseMismatch  = "mismatch"
	TransferCloseExpired   = "expired"

	// Exact sizes of the relayed values, fixed by the client protocol in
	// web/frontend/src/lib/transfer.ts: ristretto255 shares, an HMAC-SHA512
	// tag truncated to 32 bytes, and a link padded to 512 bytes, sealed with
	// XChaCha20-Poly1305 (24-byte nonce, 16-byte tag).
	TransferShareBytes        = 32
	TransferConfirmationBytes = 32
	TransferDeliveryBytes     = 552
)

// Transfer is the relay mailbox of one short-code hand-over, in three legs:
// the sender's offer, the receiver's answer and the sender's delivery. The
// server keeps only token hashes, public PAKE shares and ciphertext.
type Transfer struct {
	// TransferID is chosen by the sender: it is also the PAKE session id.
	TransferID        string
	Nameplate         int
	SenderTokenHash   string
	ReceiverTokenHash string
	// Offer is the sender's PAKE share.
	Offer []byte
	// AnswerShare and AnswerConfirmation are the receiver's PAKE share and
	// the tag proving it used the same code. Written together.
	AnswerShare        []byte
	AnswerConfirmation []byte
	// Delivery is the sealed link; storing it closes the transfer as done.
	Delivery    []byte
	CloseReason string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ClosedAt    *time.Time
}

// Claimed reports whether a receiver joined.
func (t *Transfer) Claimed() bool {
	return t.ReceiverTokenHash != ""
}

// Answered reports whether the receiver's answer is stored.
func (t *Transfer) Answered() bool {
	return t.AnswerShare != nil
}

// Delivered reports whether the sealed link is stored.
func (t *Transfer) Delivered() bool {
	return t.Delivery != nil
}

// Ended reports whether the transfer was closed or ran out.
func (t *Transfer) Ended(now time.Time) bool {
	return t.ClosedAt != nil || !now.Before(t.ExpiresAt)
}

// ValidTransferCloseReason reports whether a side may close with reason.
func ValidTransferCloseReason(reason string) bool {
	return reason == TransferCloseCancelled || reason == TransferCloseMismatch
}
