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

func putShare(t *testing.T, repo *pgadapter.SecretRepo, transferID, side string) {
	t.Helper()
	err := repo.PutTransferMessage(context.Background(), &domain.TransferMessage{
		TransferID: transferID,
		Side:       side,
		Phase:      domain.TransferPhaseShare,
		Data:       []byte("share"),
		CreatedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("PutTransferMessage: %v", err)
	}
}

func TestTransferEventsSignalStoredMessagesAndClosesOfTheirTransfer(t *testing.T) {
	pool := setupTestDB(t)
	repo := pgadapter.NewSecretRepo(pool)
	events := startTransferEvents(t, pool)
	now := time.Now()
	watched := createTestTransfer(t, repo, "watched", now)
	other := createTestTransfer(t, repo, "other", now)
	changed, unsubscribe := events.Subscribe(watched.TransferID)
	defer unsubscribe()

	putShare(t, repo, other.TransferID, domain.TransferSideSender)
	expectNoSignal(t, changed, "a message for another transfer")

	putShare(t, repo, watched.TransferID, domain.TransferSideSender)
	expectSignal(t, changed, "a stored message")

	if err := repo.CloseTransfer(context.Background(), watched.TransferID, domain.TransferCloseDone, time.Now()); err != nil {
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
