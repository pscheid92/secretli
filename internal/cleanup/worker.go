package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
)

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
		} else if count, err := uploadRepo.AbortExpiredUploadSessions(ctx, now, func(session *domain.UploadSession) error {
			return multipartStore.AbortMultipartUpload(ctx, domain.SecretStorageKey(session.PublicID), session.S3UploadID)
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
