package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
)

// Worker periodically cleans up expired secrets.
type Worker struct {
	interval   time.Duration
	secretRepo store.SecretRepo
	fileStore  storage.FileStore
}

// NewWorker creates a new cleanup worker.
func NewWorker(
	interval time.Duration,
	secretRepo store.SecretRepo,
	fileStore storage.FileStore,
) *Worker {
	return &Worker{
		interval:   interval,
		secretRepo: secretRepo,
		fileStore:  fileStore,
	}
}

// Run starts the cleanup loop. It blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("cleanup worker started", "interval", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("cleanup worker stopped")
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
		slog.Error("cleanup: failed to delete expired secrets", "error", err)
	} else if count > 0 {
		slog.Info("cleanup: deleted expired secrets", "count", count)
	}

	// Delete S3 objects for file secrets
	if w.fileStore != nil && len(storageKeys) > 0 {
		var s3Errors int
		for _, key := range storageKeys {
			if err := w.fileStore.Delete(ctx, key); err != nil {
				slog.Error("cleanup: failed to delete S3 object", "key", key, "error", err)
				s3Errors++
			}
		}
		if s3Errors > 0 {
			slog.Warn("cleanup: some S3 deletions failed", "failed", s3Errors, "total", len(storageKeys))
		}
	}
}
