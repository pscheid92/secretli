package cleanup

import (
	"context"
	"log/slog"
	"strconv"
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
	if count, err := w.secretRepo.DeleteExpiredRetrievalSessions(ctx); err != nil {
		slog.ErrorContext(ctx, "cleanup: expired retrieval session cleanup failed", "error", err)
		w.metrics.CleanupErrors.Inc()
	} else if count > 0 {
		slog.InfoContext(ctx, "cleanup: deleted retrieval sessions", "count", count)
	}

	beforeDelete := func(publicID string, objects []domain.SecretObject) error {
		if err := w.fileStore.Delete(ctx, "secrets/"+publicID); err != nil {
			return err
		}
		for _, object := range objects {
			if err := w.fileStore.Delete(ctx, objectStorageKey(object)); err != nil {
				return err
			}
		}
		return nil
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

func objectStorageKey(object domain.SecretObject) string {
	if object.ObjectKind == domain.ObjectKindManifest {
		return "secrets/" + object.PublicID + "/manifest"
	}
	return "secrets/" + object.PublicID + "/chunks/" + strconv.FormatInt(int64(object.ObjectIndex), 10)
}
