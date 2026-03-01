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
	beforeDelete := func(publicID string) error {
		return w.fileStore.Delete(ctx, "secrets/"+publicID)
	}

	count, err := w.secretRepo.DeleteExpired(ctx, beforeDelete)
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
