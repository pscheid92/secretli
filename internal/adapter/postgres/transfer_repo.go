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
		CreatedAt:       timestamptz(t.CreatedAt),
		ExpiresAt:       timestamptz(t.ExpiresAt),
	})
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
		NowAt:             timestamptz(now),
		Nameplate:         int32(nameplate), //nolint:gosec // validated range
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

func (r *SecretRepo) PutTransferMessage(ctx context.Context, msg *domain.TransferMessage) error {
	n, err := r.q.CreateTransferMessage(ctx, dbsqlc.CreateTransferMessageParams{
		TransferID: msg.TransferID,
		Side:       msg.Side,
		Phase:      msg.Phase,
		Data:       msg.Data,
		CreatedAt:  timestamptz(msg.CreatedAt),
	})
	if isForeignKeyError(err) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("insert transfer message: %w", err)
	}
	if n == 0 {
		return domain.ErrDuplicate
	}
	return nil
}

func (r *SecretRepo) GetTransferMessage(ctx context.Context, transferID, side, phase string) (*domain.TransferMessage, error) {
	row, err := r.q.GetTransferMessage(ctx, dbsqlc.GetTransferMessageParams{
		TransferID: transferID,
		Side:       side,
		Phase:      phase,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query transfer message: %w", err)
	}
	return &domain.TransferMessage{
		TransferID: row.TransferID,
		Side:       row.Side,
		Phase:      row.Phase,
		Data:       row.Data,
		CreatedAt:  row.CreatedAt.Time,
	}, nil
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

func transferFromRow(row dbsqlc.Transfer) *domain.Transfer {
	return &domain.Transfer{
		TransferID:        row.TransferID,
		Nameplate:         int(row.Nameplate),
		SenderTokenHash:   row.SenderTokenHash,
		ReceiverTokenHash: row.ReceiverTokenHash.String,
		State:             row.State,
		CloseReason:       row.CloseReason.String,
		CreatedAt:         row.CreatedAt.Time,
		ExpiresAt:         row.ExpiresAt.Time,
		ClaimedAt:         pointerFromTimestamp(row.ClaimedAt),
		ClosedAt:          pointerFromTimestamp(row.ClosedAt),
	}
}

func isForeignKeyError(err error) bool {
	if err, ok := errors.AsType[*pgconn.PgError](err); ok {
		return err.Code == "23503"
	}
	return false
}
