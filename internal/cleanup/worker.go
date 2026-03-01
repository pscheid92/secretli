package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/adapter/metrics"
)

// Worker periodically cleans up expired secrets.
type Worker struct {
	interval   time.Duration
	secretRepo domain.SecretRepo
	fileStore  domain.FileStore
	metrics    *metrics.SecretMetrics
}

// NewWorker creates a new cleanup worker.
func NewWorker(
	interval time.Duration,
	secretRepo domain.SecretRepo,
	fileStore domain.FileStore,
	m *metrics.SecretMetrics,
) *Worker {
	return &Worker{
		interval:   interval,
		secretRepo: secretRepo,
		fileStore:  fileStore,
		metrics:    m,
	}
}

// Run starts the cleanup loop. It blocks until ctx is cancelled.
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
	// Clean expired secrets
	count, storageKeys, err := w.secretRepo.DeleteExpired(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: failed to delete expired secrets", "error", err)
		if w.metrics != nil {
			w.metrics.CleanupErrors.Inc()
		}
	} else if count > 0 {
		slog.InfoContext(ctx, "cleanup: deleted expired secrets", "count", count)
		if w.metrics != nil {
			w.metrics.SecretsDeleted.WithLabelValues("cleanup").Add(float64(count))
		}
	}

	// Delete S3 objects for file secrets
	if w.fileStore != nil && len(storageKeys) > 0 {
		var s3Errors int
		for _, key := range storageKeys {
			if err := w.fileStore.Delete(ctx, key); err != nil {
				slog.ErrorContext(ctx, "cleanup: failed to delete S3 object", "key", key, "error", err)
				s3Errors++
			}
		}
		if s3Errors > 0 {
			slog.WarnContext(ctx, "cleanup: some S3 deletions failed", "failed", s3Errors, "total", len(storageKeys))
			if w.metrics != nil {
				w.metrics.CleanupErrors.Add(float64(s3Errors))
			}
		}
	}
}
