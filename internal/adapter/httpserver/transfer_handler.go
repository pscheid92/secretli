package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

const (
	// transferTTL is how long a short-code transfer stays usable.
	transferTTL = 10 * time.Minute
	// maxTransferNameplate keeps codes short; creating fails beyond it.
	maxTransferNameplate = 999
	// transferPollWait bounds one long-poll, well inside the gateway's
	// request timeout.
	transferPollWait = 25 * time.Second
	// transferRecheckInterval bounds how long a notification that never
	// arrives (the listener is down or reconnecting) can delay a long-poll.
	transferRecheckInterval = 5 * time.Second
)

// TransferEvents wakes long-polls when a transfer changes on any replica.
type TransferEvents interface {
	// Subscribe returns a channel that is signalled whenever the transfer
	// changes, and a function that ends the subscription.
	Subscribe(transferID string) (<-chan struct{}, func())
}

// noTransferEvents never signals: long-polls only re-check.
type noTransferEvents struct{}

func (noTransferEvents) Subscribe(string) (<-chan struct{}, func()) { return nil, func() {} }

// TransferHandler relays short-code transfers in three legs: the sender
// creates the transfer with its PAKE share (the offer), the receiver claims
// it by nameplate and answers with its share and a confirmation tag, and the
// sender delivers the sealed link. Each leg is written once with POST and
// waited for with a GET long-poll. The server never sees the code or the
// link.
type TransferHandler struct {
	repo            domain.TransferRepo
	events          TransferEvents
	pollWait        time.Duration
	recheckInterval time.Duration
}

type createTransferRequest struct {
	TransferID string `json:"transfer_id"`
	Offer      string `json:"offer"`
}

type claimTransferRequest struct {
	Nameplate int `json:"nameplate"`
}

type transferAnswerRequest struct {
	Share        string `json:"share"`
	Confirmation string `json:"confirmation"`
}

type transferDeliveryRequest struct {
	Sealed string `json:"sealed"`
}

// NewTransferHandler relays through repo. Without events, long-polls only
// notice changes when they re-check.
func NewTransferHandler(repo domain.TransferRepo, events TransferEvents) *TransferHandler {
	if events == nil {
		events = noTransferEvents{}
	}
	return &TransferHandler{
		repo:            repo,
		events:          events,
		pollWait:        transferPollWait,
		recheckInterval: transferRecheckInterval,
	}
}

// CreateTransfer opens a transfer with the sender's offer. The sender picks
// the transfer id, because its PAKE share depends on it.
func (h *TransferHandler) CreateTransfer(c echo.Context) error {
	var req createTransferRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid request body")
	}
	if !domain.ValidToken(req.TransferID) {
		return apperrors.BadRequestError("transfer_id must be 32 bytes of unpadded base64url")
	}
	offer, err := decodeTransferValue(req.Offer, domain.TransferShareBytes)
	if err != nil {
		return apperrors.BadRequestError("offer must be 32 bytes of unpadded base64url")
	}
	senderToken, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create transfer", err)
	}

	now := time.Now()
	transfer := &domain.Transfer{
		TransferID:      req.TransferID,
		SenderTokenHash: tokencrypto.TokenHash(senderToken),
		Offer:           offer,
		CreatedAt:       now,
		ExpiresAt:       now.Add(transferTTL),
	}
	err = h.repo.CreateTransfer(c.Request().Context(), transfer, maxTransferNameplate, now)
	if errors.Is(err, domain.ErrConflict) {
		return apperrors.UnavailableError("too many active transfers, try again shortly")
	}
	if errors.Is(err, domain.ErrDuplicate) {
		return apperrors.ConflictError("transfer_id already used")
	}
	if err != nil {
		return apperrors.InternalError("failed to create transfer", err)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"nameplate":    transfer.Nameplate,
		"sender_token": senderToken,
		"expires_at":   transfer.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// ClaimTransfer joins the transfer under a nameplate and returns the offer
// the receiver answers.
func (h *TransferHandler) ClaimTransfer(c echo.Context) error {
	var req claimTransferRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid request body")
	}
	if req.Nameplate < 1 || req.Nameplate > maxTransferNameplate {
		return apperrors.BadRequestError("invalid nameplate")
	}
	receiverToken, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to claim transfer", err)
	}

	transfer, err := h.repo.ClaimTransfer(c.Request().Context(), req.Nameplate, tokencrypto.TokenHash(receiverToken), time.Now())
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("no active transfer with this nameplate")
	}
	if errors.Is(err, domain.ErrConflict) {
		return apperrors.ConflictError("transfer already claimed")
	}
	if err != nil {
		return apperrors.InternalError("failed to claim transfer", err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"transfer_id":    transfer.TransferID,
		"receiver_token": receiverToken,
		"offer":          encodeTransferValue(transfer.Offer),
		"expires_at":     transfer.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// PostAnswer stores the receiver's PAKE share and confirmation tag. An
// identical retry succeeds; a different answer is refused.
func (h *TransferHandler) PostAnswer(c echo.Context) error {
	transfer, err := h.authenticate(c, domain.TransferSideReceiver)
	if err != nil {
		return err
	}
	var req transferAnswerRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid request body")
	}
	share, err := decodeTransferValue(req.Share, domain.TransferShareBytes)
	if err != nil {
		return apperrors.BadRequestError("share must be 32 bytes of unpadded base64url")
	}
	confirmation, err := decodeTransferValue(req.Confirmation, domain.TransferConfirmationBytes)
	if err != nil {
		return apperrors.BadRequestError("confirmation must be 32 bytes of unpadded base64url")
	}

	return h.writeOnce(c, transfer,
		func(t *domain.Transfer) (bool, bool) {
			return t.Answered(), bytes.Equal(t.AnswerShare, share) && bytes.Equal(t.AnswerConfirmation, confirmation)
		},
		func(ctx context.Context, now time.Time) (bool, error) {
			return h.repo.AnswerTransfer(ctx, transfer.TransferID, share, confirmation, now)
		},
	)
}

// AwaitAnswer long-polls for the receiver's answer.
func (h *TransferHandler) AwaitAnswer(c echo.Context) error {
	transfer, err := h.authenticate(c, domain.TransferSideSender)
	if err != nil {
		return err
	}
	answered, err := h.await(c.Request().Context(), transfer.TransferID, (*domain.Transfer).Answered)
	if answered == nil || err != nil {
		return h.awaitFailed(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"share":        encodeTransferValue(answered.AnswerShare),
		"confirmation": encodeTransferValue(answered.AnswerConfirmation),
	})
}

// PostDelivery stores the sealed link once the transfer is answered, which
// closes it as done. An identical retry succeeds.
func (h *TransferHandler) PostDelivery(c echo.Context) error {
	transfer, err := h.authenticate(c, domain.TransferSideSender)
	if err != nil {
		return err
	}
	var req transferDeliveryRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid request body")
	}
	sealed, err := decodeTransferValue(req.Sealed, domain.TransferDeliveryBytes)
	if err != nil {
		return apperrors.BadRequestError("sealed must be 552 bytes of unpadded base64url")
	}

	return h.writeOnce(c, transfer,
		func(t *domain.Transfer) (bool, bool) {
			return t.Delivered(), bytes.Equal(t.Delivery, sealed)
		},
		func(ctx context.Context, now time.Time) (bool, error) {
			return h.repo.DeliverTransfer(ctx, transfer.TransferID, sealed, now)
		},
	)
}

// AwaitDelivery long-polls for the sealed link. It is returned even though
// storing it closed the transfer.
func (h *TransferHandler) AwaitDelivery(c echo.Context) error {
	transfer, err := h.authenticate(c, domain.TransferSideReceiver)
	if err != nil {
		return err
	}
	delivered, err := h.await(c.Request().Context(), transfer.TransferID, (*domain.Transfer).Delivered)
	if delivered == nil || err != nil {
		return h.awaitFailed(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"sealed": encodeTransferValue(delivered.Delivery)})
}

// CloseTransfer ends a transfer early, as cancelled or as a mismatch. The
// delivery closes a successful one by itself.
func (h *TransferHandler) CloseTransfer(c echo.Context) error {
	transfer, err := h.authenticate(c, "")
	if err != nil {
		return err
	}
	reason := c.QueryParam("reason")
	if reason == "" {
		reason = domain.TransferCloseCancelled
	}
	if !domain.ValidTransferCloseReason(reason) {
		return apperrors.BadRequestError("invalid reason")
	}

	err = h.repo.CloseTransfer(c.Request().Context(), transfer.TransferID, reason, time.Now())
	// Closing an ended transfer is a no-op, so both sides may close.
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return apperrors.InternalError("failed to close transfer", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// writeOnce stores a leg unless it is already there. written reports
// whether the leg exists and whether it equals this request's value; store
// writes it and reports whether it did. A write can lose a race against a
// close or another write, so a refused one is judged again on fresh state.
func (h *TransferHandler) writeOnce(
	c echo.Context,
	transfer *domain.Transfer,
	written func(*domain.Transfer) (exists, same bool),
	store func(context.Context, time.Time) (bool, error),
) error {
	ctx := c.Request().Context()
	for attempt := 0; ; attempt++ {
		if exists, same := written(transfer); exists {
			if same {
				return c.NoContent(http.StatusNoContent)
			}
			return apperrors.ConflictError("already written")
		}
		now := time.Now()
		if transfer.Ended(now) {
			return transferGone(transfer)
		}
		stored, err := store(ctx, now)
		if err != nil {
			return apperrors.InternalError("failed to store transfer", err)
		}
		if stored {
			return c.NoContent(http.StatusNoContent)
		}
		if attempt > 0 {
			// Neither written nor ended: the leg before it is missing.
			return apperrors.ConflictError("the other side has not written its part yet")
		}
		transfer, err = h.repo.GetTransfer(ctx, transfer.TransferID)
		if errors.Is(err, domain.ErrNotFound) {
			return apperrors.GoneError("transfer has ended", map[string]any{"reason": domain.TransferCloseExpired})
		}
		if err != nil {
			return apperrors.InternalError("failed to read transfer", err)
		}
	}
}

// await long-polls until ready reports true for the transfer and returns
// it. It returns nil when the poll window passed first, and an error when
// the transfer ended without it or the client went away.
func (h *TransferHandler) await(ctx context.Context, transferID string, ready func(*domain.Transfer) bool) (*domain.Transfer, error) {
	// Subscribed before the first read, so a change between the read and the
	// wait still wakes this poll.
	changed, unsubscribe := h.events.Subscribe(transferID)
	defer unsubscribe()
	deadline := time.Now().Add(h.pollWait)
	for {
		transfer, err := h.repo.GetTransfer(ctx, transferID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperrors.GoneError("transfer has ended", map[string]any{"reason": domain.TransferCloseExpired})
		}
		if err != nil {
			return nil, apperrors.InternalError("failed to read transfer", err)
		}
		// A leg that was written is returned even after the transfer closed.
		if ready(transfer) {
			return transfer, nil
		}
		if transfer.Ended(time.Now()) {
			return nil, transferGone(transfer)
		}
		if !time.Now().Before(deadline) {
			return nil, nil
		}

		// A notification wakes the poll. NOTIFY is at-most-once, so the
		// re-check covers one that never arrives. Expiry ends the wait too,
		// so the gone answer isn't late.
		timer := time.NewTimer(min(h.recheckInterval, time.Until(deadline), time.Until(transfer.ExpiresAt)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// awaitFailed answers a long-poll that ended without its leg.
func (h *TransferHandler) awaitFailed(c echo.Context, err error) error {
	switch {
	case err == nil:
		return c.NoContent(http.StatusNoContent)
	case errors.Is(err, context.Canceled):
		// The client went away; there is nobody to answer.
		return nil
	default:
		return err
	}
}

// authenticate resolves the transfer and checks that the bearer token is
// the given side's; an empty side accepts either.
func (h *TransferHandler) authenticate(c echo.Context, side string) (*domain.Transfer, error) {
	transferID := c.Param("transferID")
	if !domain.ValidToken(transferID) {
		return nil, apperrors.BadRequestError("malformed transfer_id")
	}
	token, err := bearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return nil, apperrors.BadRequestError(err.Error())
	}
	if !domain.ValidToken(token) {
		return nil, apperrors.BadRequestError("malformed Authorization header")
	}

	transfer, err := h.repo.GetTransfer(c.Request().Context(), transferID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperrors.NotFoundError("transfer not found")
	}
	if err != nil {
		return nil, apperrors.InternalError("failed to read transfer", err)
	}

	hash := tokencrypto.TokenHash(token)
	var tokenSide string
	switch {
	case tokencrypto.TokensEqual(hash, transfer.SenderTokenHash):
		tokenSide = domain.TransferSideSender
	case transfer.Claimed() && tokencrypto.TokensEqual(hash, transfer.ReceiverTokenHash):
		tokenSide = domain.TransferSideReceiver
	default:
		return nil, apperrors.ForbiddenError("invalid transfer token")
	}
	if side != "" && side != tokenSide {
		return nil, apperrors.ForbiddenError("only the " + side + " does this")
	}
	return transfer, nil
}

func decodeTransferValue(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(decoded) != size {
		return nil, errors.New("wrong size")
	}
	return decoded, nil
}

func encodeTransferValue(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func transferGone(transfer *domain.Transfer) error {
	reason := transfer.CloseReason
	if transfer.ClosedAt == nil || reason == "" {
		reason = domain.TransferCloseExpired
	}
	return apperrors.GoneError("transfer has ended", map[string]any{"reason": reason})
}
