package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// transferEventsChannel matches the channel the triggers of migration 006
// notify on.
const transferEventsChannel = "transfer_events"

const (
	listenRetryMin = 500 * time.Millisecond
	// listenRetryMax keeps the listener back within seconds of the database,
	// matching the long-polls' re-check.
	listenRetryMax = 5 * time.Second
)

// TransferEvents wakes long-polls waiting on a transfer when it changes on
// any replica. Database triggers send the transfer_id with NOTIFY whenever
// a message is stored or the transfer closes; one connection per process
// LISTENs and signals the waiters for that transfer.
type TransferEvents struct {
	pool *pgxpool.Pool

	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

func NewTransferEvents(pool *pgxpool.Pool) *TransferEvents {
	return &TransferEvents{pool: pool, waiters: map[string]map[chan struct{}]struct{}{}}
}

// Subscribe returns a channel that is signalled whenever the transfer
// changes, and a function that ends the subscription. Signals coalesce, so
// a woken reader looks at the database to see what changed.
func (e *TransferEvents) Subscribe(transferID string) (<-chan struct{}, func()) {
	signal := make(chan struct{}, 1)

	e.mu.Lock()
	defer e.mu.Unlock()
	waiting := e.waiters[transferID]
	if waiting == nil {
		waiting = map[chan struct{}]struct{}{}
		e.waiters[transferID] = waiting
	}
	waiting[signal] = struct{}{}

	return signal, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.waiters[transferID], signal)
		if len(e.waiters[transferID]) == 0 {
			delete(e.waiters, transferID)
		}
	}
}

// Run listens until ctx ends. A dropped connection is re-established with
// a growing delay.
func (e *TransferEvents) Run(ctx context.Context) {
	retry := listenRetryMin
	for {
		listened, err := e.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		if listened {
			retry = listenRetryMin
		}
		slog.Warn("transfer events: not listening, long-polls rely on their re-check",
			"error", err, "retry_in", retry)

		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if !listened {
			retry = min(2*retry, listenRetryMax)
		}
	}
}

// listen holds one connection that LISTENs until it fails, and reports
// whether it got as far as listening.
func (e *TransferEvents) listen(ctx context.Context) (bool, error) {
	pooled, err := e.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire connection: %w", err)
	}
	// LISTEN binds the session for good: take the connection out of the
	// pool, so it neither counts against nor goes back to it.
	conn := pooled.Hijack()
	defer func() { _ = conn.Close(context.Background()) }()

	if _, err := conn.Exec(ctx, "LISTEN "+transferEventsChannel); err != nil {
		return false, fmt.Errorf("listen: %w", err)
	}
	slog.Info("transfer events: listening")
	// Changes made while nobody listened sent no notification here: let
	// every waiter look again.
	e.wakeAll()

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, fmt.Errorf("wait for notification: %w", err)
		}
		e.wake(notification.Payload)
	}
}

func (e *TransferEvents) wake(transferID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for signal := range e.waiters[transferID] {
		notify(signal)
	}
}

func (e *TransferEvents) wakeAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, waiting := range e.waiters {
		for signal := range waiting {
			notify(signal)
		}
	}
}

// notify signals without blocking: a pending signal already says enough.
func notify(signal chan struct{}) {
	select {
	case signal <- struct{}{}:
	default:
	}
}
