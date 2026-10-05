package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
)

func newTestTransfer(id string, now time.Time) *domain.Transfer {
	return &domain.Transfer{
		TransferID:      id,
		SenderTokenHash: tokencrypto.TokenHash("sender-" + id),
		State:           domain.TransferStateOpen,
		CreatedAt:       now,
		ExpiresAt:       now.Add(10 * time.Minute),
	}
}

func createTestTransfer(t *testing.T, repo *pgadapter.SecretRepo, id string, now time.Time) *domain.Transfer {
	t.Helper()
	transfer := newTestTransfer(id, now)
	if err := repo.CreateTransfer(context.Background(), transfer, 999, now); err != nil {
		t.Fatalf("CreateTransfer(%s): %v", id, err)
	}
	return transfer
}

func TestTransferConcurrentCreatesNeverShareANameplate(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	now := time.Now()

	const n = 20
	nameplates := make([]int, n)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			transfer := newTestTransfer(fmt.Sprintf("transfer-%02d", i), now)
			if err := repo.CreateTransfer(context.Background(), transfer, 999, now); err != nil {
				errs <- err
				return
			}
			nameplates[i] = transfer.Nameplate
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("CreateTransfer: %v", err)
	}

	sort.Ints(nameplates)
	for i, got := range nameplates {
		if got != i+1 {
			t.Fatalf("nameplates = %v, want 1..%d, each once", nameplates, n)
		}
	}
}

func TestTransferNameplatesFreeUpWhenATransferEndsOrExpires(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()

	closed := createTestTransfer(t, repo, "closed", now)
	expiring := createTestTransfer(t, repo, "expiring", now)
	if closed.Nameplate != 1 || expiring.Nameplate != 2 {
		t.Fatalf("nameplates = %d, %d, want 1, 2", closed.Nameplate, expiring.Nameplate)
	}
	if err := repo.CloseTransfer(ctx, closed.TransferID, domain.TransferCloseDone, now); err != nil {
		t.Fatalf("CloseTransfer: %v", err)
	}

	if next := createTestTransfer(t, repo, "after-close", now); next.Nameplate != 1 {
		t.Errorf("nameplate = %d, want 1 after the first transfer closed", next.Nameplate)
	}

	// Past every transfer's expiry, before cleanup has run: their numbers
	// are free again.
	later := now.Add(11 * time.Minute)
	if next := createTestTransfer(t, repo, "after-expiry", later); next.Nameplate != 1 {
		t.Errorf("nameplate = %d, want 1 once the old transfers expired", next.Nameplate)
	}
	got, err := repo.GetTransfer(ctx, expiring.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if got.State != domain.TransferStateClosed || got.CloseReason != domain.TransferCloseExpired {
		t.Errorf("expired transfer state = %q (%q), want closed (expired)", got.State, got.CloseReason)
	}
}

func TestTransferCreateFailsWhenEveryNameplateIsTaken(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	now := time.Now()
	for i := range 3 {
		if err := repo.CreateTransfer(context.Background(), newTestTransfer(fmt.Sprintf("t%d", i), now), 3, now); err != nil {
			t.Fatalf("CreateTransfer: %v", err)
		}
	}

	err := repo.CreateTransfer(context.Background(), newTestTransfer("one-too-many", now), 3, now)

	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestTransferClaimSucceedsOnce(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()
	transfer := createTestTransfer(t, repo, "claim-me", now)

	claimed, err := repo.ClaimTransfer(ctx, transfer.Nameplate, "receiver-hash", now)
	if err != nil {
		t.Fatalf("ClaimTransfer: %v", err)
	}
	if claimed.TransferID != transfer.TransferID || claimed.ReceiverTokenHash != "receiver-hash" || claimed.State != domain.TransferStateClaimed {
		t.Errorf("claimed = %+v", claimed)
	}

	if _, err := repo.ClaimTransfer(ctx, transfer.Nameplate, "attacker-hash", now); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second claim: err = %v, want ErrConflict", err)
	}
	if _, err := repo.ClaimTransfer(ctx, 42, "receiver-hash", now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown nameplate: err = %v, want ErrNotFound", err)
	}
}

func TestTransferClaimRejectsAnExpiredTransfer(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	now := time.Now()
	transfer := createTestTransfer(t, repo, "expired", now)

	_, err := repo.ClaimTransfer(context.Background(), transfer.Nameplate, "receiver-hash", now.Add(11*time.Minute))

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestTransferMessagesAreWriteOnceAndNeedATransfer(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()
	transfer := createTestTransfer(t, repo, "messages", now)
	msg := &domain.TransferMessage{
		TransferID: transfer.TransferID,
		Side:       domain.TransferSideSender,
		Phase:      domain.TransferPhaseShare,
		Data:       []byte{1, 2, 3},
		CreatedAt:  now,
	}

	if err := repo.PutTransferMessage(ctx, msg); err != nil {
		t.Fatalf("PutTransferMessage: %v", err)
	}
	got, err := repo.GetTransferMessage(ctx, transfer.TransferID, domain.TransferSideSender, domain.TransferPhaseShare)
	if err != nil || string(got.Data) != string(msg.Data) {
		t.Fatalf("GetTransferMessage = %v, %v", got, err)
	}

	rewrite := *msg
	rewrite.Data = []byte{9}
	if err := repo.PutTransferMessage(ctx, &rewrite); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("rewrite: err = %v, want ErrDuplicate", err)
	}
	orphan := *msg
	orphan.TransferID = "no-such-transfer"
	if err := repo.PutTransferMessage(ctx, &orphan); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("orphan message: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetTransferMessage(ctx, transfer.TransferID, domain.TransferSideReceiver, domain.TransferPhaseShare); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing message: err = %v, want ErrNotFound", err)
	}
}

func TestTransferCloseRecordsTheReasonOnce(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()
	transfer := createTestTransfer(t, repo, "close-me", now)

	if err := repo.CloseTransfer(ctx, transfer.TransferID, domain.TransferCloseMismatch, now); err != nil {
		t.Fatalf("CloseTransfer: %v", err)
	}
	if err := repo.CloseTransfer(ctx, transfer.TransferID, domain.TransferCloseDone, now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second close: err = %v, want ErrNotFound", err)
	}
	got, err := repo.GetTransfer(ctx, transfer.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if got.State != domain.TransferStateClosed || got.CloseReason != domain.TransferCloseMismatch || got.ClosedAt == nil {
		t.Errorf("transfer = %+v, want closed with the first reason", got)
	}
}

func TestTransferCleanupDeletesEndedTransfersWithTheirMessages(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()

	active := createTestTransfer(t, repo, "active", now)
	closed := createTestTransfer(t, repo, "closed", now)
	expired := newTestTransfer("expired", now.Add(-20*time.Minute))
	if err := repo.CreateTransfer(ctx, expired, 999, now.Add(-20*time.Minute)); err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	if err := repo.PutTransferMessage(ctx, &domain.TransferMessage{TransferID: closed.TransferID, Side: domain.TransferSideSender, Phase: domain.TransferPhaseShare, Data: []byte{1}, CreatedAt: now}); err != nil {
		t.Fatalf("PutTransferMessage: %v", err)
	}
	if err := repo.CloseTransfer(ctx, closed.TransferID, domain.TransferCloseDone, now.Add(-2*time.Minute)); err != nil {
		t.Fatalf("CloseTransfer: %v", err)
	}

	deleted, err := repo.DeleteEndedTransfers(ctx, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("DeleteEndedTransfers: %v", err)
	}

	if deleted != 2 {
		t.Errorf("deleted = %d, want the closed and the expired transfer", deleted)
	}
	if _, err := repo.GetTransfer(ctx, active.TransferID); err != nil {
		t.Errorf("active transfer: %v, want it kept", err)
	}
	if _, err := repo.GetTransferMessage(ctx, closed.TransferID, domain.TransferSideSender, domain.TransferPhaseShare); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("message of deleted transfer: err = %v, want ErrNotFound", err)
	}
}
