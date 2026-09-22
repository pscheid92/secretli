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
)

// Repo is the slice of the datastore this worker touches.
type Repo interface {
	DeleteExpired(ctx context.Context, now time.Time, beforeDelete func(storageKey string) error) (int64, error)
	DeleteExpiredRetrievalSessions(ctx context.Context, now time.Time) (int64, error)
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, beforeAbort func(session *domain.UploadSession) error) (int64, error)
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

	if count, err := w.secretRepo.AbortExpiredUploadSessions(ctx, now, func(session *domain.UploadSession) error {
		if err := w.fileStore.AbortMultipartUpload(ctx, session.StorageKey, session.S3UploadID); err != nil {
			return err
		}
		// A crash between storage completion and the database commit leaves a
		// finished object with no secret row. The key belongs to this session
		// alone, so remove it rather than leaking storage.
		return w.fileStore.Delete(ctx, session.StorageKey)
	}); err != nil {
		slog.ErrorContext(ctx, "cleanup: expired upload session cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	} else if count > 0 {
		slog.InfoContext(ctx, "cleanup: aborted expired upload sessions", "count", count)
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

	count, err := w.secretRepo.DeleteExpired(ctx, now, beforeDelete)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: cycle failed", "error", err)
		w.metrics.CleanupErrors.Inc()
		return
	}

	if count > 0 {
		slog.InfoContext(ctx, "cleanup: deleted secrets", "count", count)
		w.metrics.SecretsDeleted.WithLabelValues("cleanup").Add(float64(count))
	}
}
