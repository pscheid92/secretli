package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

const (
	// cycleTimeout bounds one cleanup pass. The repo holds row locks while the
	// storage callbacks run, so a hung S3 call must not block API deletes
	// forever.
	cycleTimeout = 10 * time.Minute
	// finishedUploadRetention is how long a completed or aborted upload
	// session's tombstone is kept, so a client retrying complete or abort
	// still gets a consistent answer.
	finishedUploadRetention = time.Hour
	// batchSize bounds how many rows one cleanup transaction locks while it
	// makes storage calls. Each batch commits, so a large backlog (after an
	// outage, say) is worked off across batches and cycles instead of in one
	// transaction that times out and rolls back every time.
	batchSize = 100
)

// Repo is the slice of the datastore this worker touches.
type Repo interface {
	DeleteExpired(ctx context.Context, now time.Time, limit int, beforeDelete func(storageKey string) error) (domain.CleanupBatch, error)
	DeleteExpiredRetrievalSessions(ctx context.Context, now time.Time) (int64, error)
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, limit int, beforeAbort func(session *domain.UploadSession) error) (domain.CleanupBatch, error)
	DeleteFinishedUploadSessions(ctx context.Context, finishedBefore time.Time) (int64, error)
}

type Worker struct {
	interval   time.Duration
	secretRepo Repo
	fileStore  domain.MultipartFileStore
	metrics    *metrics.SecretMetrics
}

func NewWorker(interval time.Duration, secretRepo Repo, fileStore domain.MultipartFileStore, m *metrics.SecretMetrics) *Worker {
	return &Worker{
		interval:   interval,
		secretRepo: secretRepo,
		fileStore:  fileStore,
		metrics:    m,
	}
}

func (w *Worker) Run(ctx context.Context) {
	slog.InfoContext(ctx, "cleanup worker started", "interval", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "cleanup worker stopped")
			return
		case <-ticker.C:
			w.runCycle(ctx)
		}
	}
}

func (w *Worker) runCycle(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, cycleTimeout)
	defer cancel()
	now := time.Now()

	if count, err := w.secretRepo.DeleteExpiredRetrievalSessions(ctx, now); err != nil {
		slog.ErrorContext(ctx, "cleanup: expired retrieval session cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	} else if count > 0 {
		slog.InfoContext(ctx, "cleanup: deleted retrieval sessions", "count", count)
	}

	abortUpload := func(session *domain.UploadSession) error {
		if err := w.fileStore.AbortMultipartUpload(ctx, session.StorageKey, session.S3UploadID); err != nil {
			return err
		}
		// A crash between storage completion and the database commit leaves a
		// finished object with no secret row. The key belongs to this session
		// alone, so remove it rather than leaking storage.
		return w.fileStore.Delete(ctx, session.StorageKey)
	}
	aborted, err := drainBatches(ctx, func() (domain.CleanupBatch, error) {
		return w.secretRepo.AbortExpiredUploadSessions(ctx, now, batchSize, abortUpload)
	})
	if aborted > 0 {
		slog.InfoContext(ctx, "cleanup: aborted expired upload sessions", "count", aborted)
	}
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: expired upload session cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	}

	if count, err := w.secretRepo.DeleteFinishedUploadSessions(ctx, now.Add(-finishedUploadRetention)); err != nil {
		slog.ErrorContext(ctx, "cleanup: finished upload session cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	} else if count > 0 {
		slog.InfoContext(ctx, "cleanup: deleted finished upload sessions", "count", count)
	}

	beforeDelete := func(storageKey string) error {
		return w.fileStore.Delete(ctx, storageKey)
	}
	deleted, err := drainBatches(ctx, func() (domain.CleanupBatch, error) {
		return w.secretRepo.DeleteExpired(ctx, now, batchSize, beforeDelete)
	})
	if deleted > 0 {
		slog.InfoContext(ctx, "cleanup: deleted secrets", "count", deleted)
		w.metrics.SecretsDeleted.WithLabelValues("cleanup").Add(float64(deleted))
	}
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: secret cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	}
}

// drainBatches runs batch until the backlog is worked off and returns how
// many rows were removed. It stops at a short batch (nothing left), at a
// batch that removed nothing (every row failed, e.g. storage is down; the
// next cycle retries) or at an error, whose earlier batches stay committed.
// Rows that fail are picked up again by the next batch alongside new ones,
// so a few bad rows cannot stall the rest. Every batch either removes a row
// or ends the loop, and the set of due rows is fixed by now, so it ends.
func drainBatches(ctx context.Context, batch func() (domain.CleanupBatch, error)) (int, error) {
	removed := 0
	for ctx.Err() == nil {
		result, err := batch()
		removed += result.Removed
		if err != nil {
			return removed, err
		}
		if result.Found < batchSize || result.Removed == 0 {
			break
		}
	}
	return removed, nil
}
