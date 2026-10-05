package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/pscheid92/secretli/internal/domain"
)

// listenerPID is the backend that LISTENs for transfer events, or 0.
func listenerPID(t *testing.T, pool *pgxpool.Pool) int32 {
	t.Helper()
	var pid int32
	err := pool.QueryRow(context.Background(),
		"SELECT COALESCE(MAX(pid), 0) FROM pg_stat_activity WHERE query = 'LISTEN transfer_events'").Scan(&pid)
	if err != nil {
		t.Fatalf("find listener: %v", err)
	}
	return pid
}

// startTransferEvents runs the listener until the test ends and returns once
// the database sees it listen.
func startTransferEvents(t *testing.T, pool *pgxpool.Pool) *pgadapter.TransferEvents {
	t.Helper()
	events := pgadapter.NewTransferEvents(pool)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		events.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		// Gone before the next test looks for its own listener.
		waitUntil(t, "the listener is gone", func() bool { return listenerPID(t, pool) == 0 })
	})
	waitUntil(t, "listening", func() bool { return listenerPID(t, pool) != 0 })
	return events
}

func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func expectSignal(t *testing.T, changed <-chan struct{}, after string) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatalf("no signal after %s", after)
	}
}

func expectNoSignal(t *testing.T, changed <-chan struct{}, after string) {
	t.Helper()
	select {
	case <-changed:
		t.Fatalf("unexpected signal after %s", after)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestTransferEventsSignalEveryChangeOfTheirTransfer(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	events := startTransferEvents(t, pool)
	now := time.Now()
	watched := createTestTransfer(t, repo, "watched", now)
	other := createTestTransfer(t, repo, "other", now)
	changed, unsubscribe := events.Subscribe(watched.TransferID)
	defer unsubscribe()

	claimTestTransfer(t, repo, other, now)
	answerTestTransfer(t, repo, other, now)
	expectNoSignal(t, changed, "changes of another transfer")

	claimTestTransfer(t, repo, watched, now)
	expectSignal(t, changed, "the claim")
	answerTestTransfer(t, repo, watched, now)
	expectSignal(t, changed, "the answer")
	if stored, err := repo.DeliverTransfer(context.Background(), watched.TransferID, testDelivery, now); err != nil || !stored {
		t.Fatalf("DeliverTransfer = %v, %v", stored, err)
	}
	expectSignal(t, changed, "the delivery")
}

func TestTransferEventsSignalAClose(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	events := startTransferEvents(t, pool)
	transfer := createTestTransfer(t, repo, "closing", time.Now())
	changed, unsubscribe := events.Subscribe(transfer.TransferID)
	defer unsubscribe()

	if err := repo.CloseTransfer(context.Background(), transfer.TransferID, domain.TransferCloseCancelled, time.Now()); err != nil {
		t.Fatalf("CloseTransfer: %v", err)
	}

	expectSignal(t, changed, "closing the transfer")
}

func TestTransferEventsRelistenAfterADroppedConnectionAndWakeEveryWaiter(t *testing.T) {
	pool := setupTestDB(t)
	events := startTransferEvents(t, pool)
	changed, unsubscribe := events.Subscribe("waiting")
	defer unsubscribe()
	first := listenerPID(t, pool)

	// As a database failover would.
	if _, err := pool.Exec(context.Background(), "SELECT pg_terminate_backend($1)", first); err != nil {
		t.Fatalf("terminate listener: %v", err)
	}

	// Whatever changed in between sent no notification here: waiters look again.
	expectSignal(t, changed, "listening again")
	waitUntil(t, "a new listener", func() bool {
		pid := listenerPID(t, pool)
		return pid != 0 && pid != first
	})
}
