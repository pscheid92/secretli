package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pscheid92/secretli/internal/adapter/postgres/dbsqlc"
	"github.com/pscheid92/secretli/internal/domain"
)

// transferPrimaryKey is the constraint a reused transfer id violates.
const transferPrimaryKey = "code_transfers_pkey"

func (r *SecretRepo) CreateTransfer(ctx context.Context, t *domain.Transfer, maxNameplate int, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create transfer tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := r.q.WithTx(tx)

	if err := qtx.LockTransferNameplates(ctx); err != nil {
		return fmt.Errorf("lock transfer nameplates: %w", err)
	}
	if err := qtx.CloseExpiredActiveTransfers(ctx, timestamptz(now)); err != nil {
		return fmt.Errorf("close expired transfers: %w", err)
	}
	nameplate, err := qtx.NextFreeNameplate(ctx, int32(maxNameplate)) //nolint:gosec // small constant chosen by the caller
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("find free nameplate: %w", err)
	}
	err = qtx.CreateTransfer(ctx, dbsqlc.CreateTransferParams{
		TransferID:      t.TransferID,
		Nameplate:       nameplate,
		SenderTokenHash: t.SenderTokenHash,
		Offer:           t.Offer,
		CreatedAt:       timestamptz(t.CreatedAt),
		ExpiresAt:       timestamptz(t.ExpiresAt),
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.ConstraintName == transferPrimaryKey {
		return domain.ErrDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create transfer: %w", err)
	}
	t.Nameplate = int(nameplate)
	return nil
}

func (r *SecretRepo) ClaimTransfer(ctx context.Context, nameplate int, receiverTokenHash string, now time.Time) (*domain.Transfer, error) {
	row, err := r.q.ClaimTransfer(ctx, dbsqlc.ClaimTransferParams{
		ReceiverTokenHash: text(receiverTokenHash),
		Nameplate:         int32(nameplate), //nolint:gosec // validated range
		NowAt:             timestamptz(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		claimed, err := r.q.ClaimedTransferExists(ctx, dbsqlc.ClaimedTransferExistsParams{
			Nameplate: int32(nameplate), //nolint:gosec // validated range
			NowAt:     timestamptz(now),
		})
		if err != nil {
			return nil, fmt.Errorf("check claimed transfer: %w", err)
		}
		if claimed {
			return nil, domain.ErrConflict
		}
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("claim transfer: %w", err)
	}
	return transferFromRow(row), nil
}

func (r *SecretRepo) GetTransfer(ctx context.Context, transferID string) (*domain.Transfer, error) {
	row, err := r.q.GetTransfer(ctx, transferID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query transfer: %w", err)
	}
	return transferFromRow(row), nil
}

func (r *SecretRepo) AnswerTransfer(ctx context.Context, transferID string, share, confirmation []byte, now time.Time) (bool, error) {
	n, err := r.q.AnswerTransfer(ctx, dbsqlc.AnswerTransferParams{
		AnswerShare:        share,
		AnswerConfirmation: confirmation,
		TransferID:         transferID,
		NowAt:              timestamptz(now),
	})
	if err != nil {
		return false, fmt.Errorf("answer transfer: %w", err)
	}
	return n == 1, nil
}

func (r *SecretRepo) DeliverTransfer(ctx context.Context, transferID string, delivery []byte, now time.Time) (bool, error) {
	n, err := r.q.DeliverTransfer(ctx, dbsqlc.DeliverTransferParams{
		Delivery:   delivery,
		NowAt:      timestamptz(now),
		TransferID: transferID,
	})
	if err != nil {
		return false, fmt.Errorf("deliver transfer: %w", err)
	}
	return n == 1, nil
}

func (r *SecretRepo) CloseTransfer(ctx context.Context, transferID, reason string, now time.Time) error {
	n, err := r.q.CloseTransfer(ctx, dbsqlc.CloseTransferParams{
		CloseReason: text(reason),
		NowAt:       timestamptz(now),
		TransferID:  transferID,
	})
	if err != nil {
		return fmt.Errorf("close transfer: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SecretRepo) DeleteEndedTransfers(ctx context.Context, endedBefore time.Time) (int64, error) {
	n, err := r.q.DeleteEndedTransfers(ctx, timestamptz(endedBefore))
	if err != nil {
		return 0, fmt.Errorf("delete ended transfers: %w", err)
	}
	return n, nil
}

func transferFromRow(row dbsqlc.CodeTransfer) *domain.Transfer {
	return &domain.Transfer{
		TransferID:         row.TransferID,
		Nameplate:          int(row.Nameplate),
		SenderTokenHash:    row.SenderTokenHash,
		ReceiverTokenHash:  row.ReceiverTokenHash.String,
		Offer:              row.Offer,
		AnswerShare:        row.AnswerShare,
		AnswerConfirmation: row.AnswerConfirmation,
		Delivery:           row.Delivery,
		CloseReason:        row.CloseReason.String,
		CreatedAt:          row.CreatedAt.Time,
		ExpiresAt:          row.ExpiresAt.Time,
		ClosedAt:           pointerFromTimestamp(row.ClosedAt),
	}
}
