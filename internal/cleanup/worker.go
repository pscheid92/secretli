package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

// cycleTimeout bounds one cleanup pass. The repo holds row locks while the
// storage callbacks run, so a hung S3 call must not block API deletes forever.
const cycleTimeout = 10 * time.Minute

// Repo is the slice of the datastore this worker touches.
type Repo interface {
	DeleteExpired(ctx context.Context, now time.Time, beforeDelete func(publicID string) error) (int64, error)
	DeleteExpiredRetrievalSessions(ctx context.Context, now time.Time) (int64, error)
	AbortExpiredUploadSessions(ctx context.Context, now time.Time, beforeAbort func(session *domain.UploadSession, secretExists bool) error) (int64, error)
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

	if count, err := w.secretRepo.AbortExpiredUploadSessions(ctx, now, func(session *domain.UploadSession, secretExists bool) error {
		key := domain.SecretStorageKey(session.PublicID)
		if err := w.fileStore.AbortMultipartUpload(ctx, key, session.S3UploadID); err != nil {
			return err
		}
		// A crash between storage completion and the database commit leaves a
		// finished object with no secret row. Nothing can ever serve it, so
		// remove it rather than leaking storage.
		if !secretExists {
			return w.fileStore.Delete(ctx, key)
		}
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "cleanup: expired upload session cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	} else if count > 0 {
		slog.InfoContext(ctx, "cleanup: aborted expired upload sessions", "count", count)
	}

	beforeDelete := func(publicID string) error {
		return w.fileStore.Delete(ctx, domain.SecretStorageKey(publicID))
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
