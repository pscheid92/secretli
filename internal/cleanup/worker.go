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

type Worker struct {
	interval   time.Duration
	secretRepo domain.SecretRepo
	fileStore  domain.FileStore
	metrics    *metrics.SecretMetrics
}

func NewWorker(interval time.Duration, secretRepo domain.SecretRepo, fileStore domain.FileStore, m *metrics.SecretMetrics) *Worker {
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

	if uploadRepo, ok := w.secretRepo.(domain.UploadSessionCleanupRepo); ok {
		multipartStore, ok := w.fileStore.(domain.MultipartFileStore)
		if !ok {
			slog.ErrorContext(ctx, "cleanup: multipart upload cleanup unavailable")
			w.metrics.CleanupErrors.Inc()
		} else if count, err := uploadRepo.AbortExpiredUploadSessions(ctx, now, func(session *domain.UploadSession, secretExists bool) error {
			key := domain.SecretStorageKey(session.PublicID)
			if err := multipartStore.AbortMultipartUpload(ctx, key, session.S3UploadID); err != nil {
				return err
			}
			// A crash between storage completion and the database commit leaves
			// a finished object with no secret row. Nothing can ever serve it,
			// so remove it rather than leaking storage.
			if !secretExists {
				return multipartStore.Delete(ctx, key)
			}
			return nil
		}); err != nil {
			slog.ErrorContext(ctx, "cleanup: expired upload session cleanup failed", "error", err)
			w.metrics.CleanupErrors.Inc()
		} else if count > 0 {
			slog.InfoContext(ctx, "cleanup: aborted expired upload sessions", "count", count)
		}
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
