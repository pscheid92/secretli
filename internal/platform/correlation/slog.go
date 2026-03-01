package correlation

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5/middleware"
)

type Handler struct {
	inner slog.Handler
}

func NewHandler(inner slog.Handler) *Handler {
	return &Handler{inner: inner}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if id := middleware.GetReqID(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	return h.inner.Handle(ctx, record)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{inner: h.inner.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: h.inner.WithGroup(name)}
}
