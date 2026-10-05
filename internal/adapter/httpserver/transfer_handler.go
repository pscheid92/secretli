package httpserver

import (
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

// TransferHandler relays short-code transfers: two browsers exchange PAKE
// shares, a confirmation tag and the sealed link through write-once
// messages. It never sees the code or the link.
type TransferHandler struct {
	repo            domain.TransferRepo
	events          TransferEvents
	pollWait        time.Duration
	recheckInterval time.Duration
}

type claimTransferRequest struct {
	Nameplate int `json:"nameplate"`
}

type transferMessageRequest struct {
	Data string `json:"data"`
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

func (h *TransferHandler) CreateTransfer(c echo.Context) error {
	transferID, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create transfer", err)
	}
	senderToken, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create transfer", err)
	}

	now := time.Now()
	transfer := &domain.Transfer{
		TransferID:      transferID,
		SenderTokenHash: tokencrypto.TokenHash(senderToken),
		State:           domain.TransferStateOpen,
		CreatedAt:       now,
		ExpiresAt:       now.Add(transferTTL),
	}
	err = h.repo.CreateTransfer(c.Request().Context(), transfer, maxTransferNameplate, now)
	if errors.Is(err, domain.ErrConflict) {
		return apperrors.UnavailableError("too many active transfers, try again shortly")
	}
	if err != nil {
		return apperrors.InternalError("failed to create transfer", err)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"nameplate":    transfer.Nameplate,
		"transfer_id":  transfer.TransferID,
		"sender_token": senderToken,
		"expires_at":   transfer.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

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
		"expires_at":     transfer.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *TransferHandler) PutMessage(c echo.Context) error {
	transfer, side, err := h.authenticate(c)
	if err != nil {
		return err
	}
	phase := c.Param("phase")
	if !mayWritePhase(side, phase) {
		return apperrors.BadRequestError("this side cannot write this phase")
	}
	if transfer.Ended(time.Now()) {
		return transferGone(transfer)
	}

	var req transferMessageRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid request body")
	}
	data, err := base64.RawURLEncoding.DecodeString(req.Data)
	if err != nil || len(data) == 0 || len(data) > domain.TransferMessageMaxBytes {
		return apperrors.BadRequestError("data must be 1 to 4096 bytes of unpadded base64url")
	}

	err = h.repo.PutTransferMessage(c.Request().Context(), &domain.TransferMessage{
		TransferID: transfer.TransferID,
		Side:       side,
		Phase:      phase,
		Data:       data,
		CreatedAt:  time.Now(),
	})
	if errors.Is(err, domain.ErrDuplicate) {
		return apperrors.ConflictError("message already written")
	}
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.GoneError("transfer has ended", map[string]any{"reason": domain.TransferCloseExpired})
	}
	if err != nil {
		return apperrors.InternalError("failed to store transfer message", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GetMessage long-polls for the other side's message in a phase. A message
// already written is returned even after the transfer was closed, so the
// receiver can still read a payload the sender sent before finishing.
func (h *TransferHandler) GetMessage(c echo.Context) error {
	transfer, side, err := h.authenticate(c)
	if err != nil {
		return err
	}
	other := domain.OtherTransferSide(side)
	phase := c.Param("phase")
	if !mayWritePhase(other, phase) {
		return apperrors.BadRequestError("the other side never writes this phase")
	}

	ctx := c.Request().Context()
	// Subscribed before the first read, so a write between the read and the
	// wait still wakes this poll.
	changed, unsubscribe := h.events.Subscribe(transfer.TransferID)
	defer unsubscribe()
	deadline := time.Now().Add(h.pollWait)
	for {
		msg, err := h.repo.GetTransferMessage(ctx, transfer.TransferID, other, phase)
		if err == nil {
			return c.JSON(http.StatusOK, map[string]any{"data": base64.RawURLEncoding.EncodeToString(msg.Data)})
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return apperrors.InternalError("failed to read transfer message", err)
		}

		transfer, err = h.repo.GetTransfer(ctx, transfer.TransferID)
		if errors.Is(err, domain.ErrNotFound) {
			return apperrors.GoneError("transfer has ended", map[string]any{"reason": domain.TransferCloseExpired})
		}
		if err != nil {
			return apperrors.InternalError("failed to read transfer", err)
		}
		if transfer.Ended(time.Now()) {
			return transferGone(transfer)
		}
		if !time.Now().Before(deadline) {
			return c.NoContent(http.StatusNoContent)
		}

		// A notification wakes the poll. NOTIFY is at-most-once, so the
		// re-check covers one that never arrives. Expiry ends the wait too,
		// so the gone answer isn't late.
		timer := time.NewTimer(min(h.recheckInterval, time.Until(deadline), time.Until(transfer.ExpiresAt)))
		select {
		case <-ctx.Done():
			timer.Stop()
			// The client went away; there is nobody to answer.
			return nil
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (h *TransferHandler) CloseTransfer(c echo.Context) error {
	transfer, _, err := h.authenticate(c)
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

// authenticate resolves the transfer and which side the bearer token belongs to.
func (h *TransferHandler) authenticate(c echo.Context) (*domain.Transfer, string, error) {
	transferID := c.Param("transferID")
	if !domain.ValidToken(transferID) {
		return nil, "", apperrors.BadRequestError("malformed transfer_id")
	}
	token, err := bearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return nil, "", apperrors.BadRequestError(err.Error())
	}
	if !domain.ValidToken(token) {
		return nil, "", apperrors.BadRequestError("malformed Authorization header")
	}

	transfer, err := h.repo.GetTransfer(c.Request().Context(), transferID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, "", apperrors.NotFoundError("transfer not found")
	}
	if err != nil {
		return nil, "", apperrors.InternalError("failed to read transfer", err)
	}

	hash := tokencrypto.TokenHash(token)
	switch {
	case tokencrypto.TokensEqual(hash, transfer.SenderTokenHash):
		return transfer, domain.TransferSideSender, nil
	case transfer.ReceiverTokenHash != "" && tokencrypto.TokensEqual(hash, transfer.ReceiverTokenHash):
		return transfer, domain.TransferSideReceiver, nil
	default:
		return nil, "", apperrors.ForbiddenError("invalid transfer token")
	}
}

// mayWritePhase encodes the protocol: both sides send a PAKE share, only the
// receiver sends the confirmation tag and only the sender sends the payload.
func mayWritePhase(side, phase string) bool {
	switch phase {
	case domain.TransferPhaseShare:
		return true
	case domain.TransferPhaseConfirm:
		return side == domain.TransferSideReceiver
	case domain.TransferPhasePayload:
		return side == domain.TransferSideSender
	default:
		return false
	}
}

func transferGone(transfer *domain.Transfer) error {
	reason := transfer.CloseReason
	if transfer.State != domain.TransferStateClosed || reason == "" {
		reason = domain.TransferCloseExpired
	}
	return apperrors.GoneError("transfer has ended", map[string]any{"reason": reason})
}
