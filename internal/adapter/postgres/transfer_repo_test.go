package postgres_test

import (
	"bytes"
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

var (
	testOffer        = bytes.Repeat([]byte{'a'}, domain.TransferShareBytes)
	testAnswerShare  = bytes.Repeat([]byte{'b'}, domain.TransferShareBytes)
	testConfirmation = bytes.Repeat([]byte{'t'}, domain.TransferConfirmationBytes)
	testDelivery     = bytes.Repeat([]byte{'s'}, domain.TransferDeliveryBytes)
)

func newTestTransfer(id string, now time.Time) *domain.Transfer {
	return &domain.Transfer{
		TransferID:      id,
		SenderTokenHash: tokencrypto.TokenHash("sender-" + id),
		Offer:           testOffer,
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

func claimTestTransfer(t *testing.T, repo *pgadapter.SecretRepo, transfer *domain.Transfer, now time.Time) {
	t.Helper()
	if _, err := repo.ClaimTransfer(context.Background(), transfer.Nameplate, "receiver-"+transfer.TransferID, now); err != nil {
		t.Fatalf("ClaimTransfer(%s): %v", transfer.TransferID, err)
	}
}

func answerTestTransfer(t *testing.T, repo *pgadapter.SecretRepo, transfer *domain.Transfer, now time.Time) {
	t.Helper()
	stored, err := repo.AnswerTransfer(context.Background(), transfer.TransferID, testAnswerShare, testConfirmation, now)
	if err != nil || !stored {
		t.Fatalf("AnswerTransfer(%s) = %v, %v", transfer.TransferID, stored, err)
	}
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
	if err := repo.CloseTransfer(ctx, closed.TransferID, domain.TransferCloseCancelled, now); err != nil {
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
	if got.ClosedAt == nil || got.CloseReason != domain.TransferCloseExpired {
		t.Errorf("expired transfer closed at %v as %q, want closed as expired", got.ClosedAt, got.CloseReason)
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

func TestTransferCreateRefusesAReusedID(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	now := time.Now()
	createTestTransfer(t, repo, "only-once", now)

	err := repo.CreateTransfer(context.Background(), newTestTransfer("only-once", now), 999, now)

	if !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestTransferClaimSucceedsOnceAndReturnsTheOffer(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()
	transfer := createTestTransfer(t, repo, "claim-me", now)

	claimed, err := repo.ClaimTransfer(ctx, transfer.Nameplate, "receiver-hash", now)
	if err != nil {
		t.Fatalf("ClaimTransfer: %v", err)
	}
	if claimed.TransferID != transfer.TransferID || claimed.ReceiverTokenHash != "receiver-hash" || !bytes.Equal(claimed.Offer, testOffer) {
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

func TestTransferAnswerNeedsAClaimAndIsWrittenOnce(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()
	transfer := createTestTransfer(t, repo, "answer-me", now)

	if stored, err := repo.AnswerTransfer(ctx, transfer.TransferID, testAnswerShare, testConfirmation, now); err != nil || stored {
		t.Errorf("answer before the claim = %v, %v, want not stored", stored, err)
	}
	claimTestTransfer(t, repo, transfer, now)
	answerTestTransfer(t, repo, transfer, now)
	if stored, err := repo.AnswerTransfer(ctx, transfer.TransferID, testDelivery[:32], testConfirmation, now); err != nil || stored {
		t.Errorf("second answer = %v, %v, want not stored", stored, err)
	}

	got, err := repo.GetTransfer(ctx, transfer.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if !bytes.Equal(got.AnswerShare, testAnswerShare) || !bytes.Equal(got.AnswerConfirmation, testConfirmation) {
		t.Errorf("answer = %x / %x, want the first one", got.AnswerShare, got.AnswerConfirmation)
	}
}

func TestTransferDeliveryNeedsTheAnswerAndClosesAsDone(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()
	transfer := createTestTransfer(t, repo, "deliver-me", now)
	claimTestTransfer(t, repo, transfer, now)

	if stored, err := repo.DeliverTransfer(ctx, transfer.TransferID, testDelivery, now); err != nil || stored {
		t.Errorf("delivery before the answer = %v, %v, want not stored", stored, err)
	}
	answerTestTransfer(t, repo, transfer, now)
	if stored, err := repo.DeliverTransfer(ctx, transfer.TransferID, testDelivery, now); err != nil || !stored {
		t.Fatalf("DeliverTransfer = %v, %v", stored, err)
	}
	if stored, err := repo.DeliverTransfer(ctx, transfer.TransferID, testDelivery, now); err != nil || stored {
		t.Errorf("second delivery = %v, %v, want not stored", stored, err)
	}

	got, err := repo.GetTransfer(ctx, transfer.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if !bytes.Equal(got.Delivery, testDelivery) || got.CloseReason != domain.TransferCloseDone || got.ClosedAt == nil {
		t.Errorf("transfer = %+v, want delivered and closed as done", got)
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
	if err := repo.CloseTransfer(ctx, transfer.TransferID, domain.TransferCloseCancelled, now); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second close: err = %v, want ErrNotFound", err)
	}
	got, err := repo.GetTransfer(ctx, transfer.TransferID)
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if got.CloseReason != domain.TransferCloseMismatch || got.ClosedAt == nil {
		t.Errorf("transfer = %+v, want closed with the first reason", got)
	}
}

func TestTransferCleanupDeletesEndedTransfers(t *testing.T) {
	repo := pgadapter.NewSecretRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now()

	active := createTestTransfer(t, repo, "active", now)
	closed := createTestTransfer(t, repo, "closed", now)
	expired := newTestTransfer("expired", now.Add(-20*time.Minute))
	if err := repo.CreateTransfer(ctx, expired, 999, now.Add(-20*time.Minute)); err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	if err := repo.CloseTransfer(ctx, closed.TransferID, domain.TransferCloseCancelled, now.Add(-2*time.Minute)); err != nil {
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
	if _, err := repo.GetTransfer(ctx, closed.TransferID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("closed transfer: err = %v, want it deleted", err)
	}
}
