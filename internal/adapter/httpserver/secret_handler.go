package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

type SecretHandler struct {
	repo        domain.SecretRepo
	fileStore   domain.FileStore
	maxFileSize int64
	metrics     *metrics.SecretMetrics
}

func NewSecretHandler(repo domain.SecretRepo, fileStore domain.FileStore, maxFileSize int64, m *metrics.SecretMetrics) *SecretHandler {
	return &SecretHandler{repo: repo, fileStore: fileStore, maxFileSize: maxFileSize, metrics: m}
}

func (h *SecretHandler) CreateSecret(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize+1<<20)

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apperrors.BadRequestError("request exceeds maximum size limit")
		}
		return apperrors.BadRequestError("invalid multipart form")
	}

	metadataStr := r.FormValue("metadata")
	if metadataStr == "" {
		return apperrors.BadRequestError("missing metadata part")
	}

	var meta domain.CreateSecretRequest
	if err := json.Unmarshal([]byte(metadataStr), &meta); err != nil {
		return apperrors.BadRequestError("invalid metadata JSON")
	}

	if details := validateRequest(&meta); details != nil {
		return validationError(details)
	}

	duration, err := parseExpiration(meta.Expiration)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return apperrors.BadRequestError("missing file part")
	}
	defer func() { _ = file.Close() }()

	secret := &domain.Secret{
		PublicID:          meta.PublicID,
		RetrievalToken:    meta.RetrievalToken,
		DeletionToken:     meta.DeletionToken,
		EncryptedMeta:     meta.EncryptedMeta,
		BlobSize:          header.Size,
		BurnAfterRead: meta.BurnAfterRead,
		ExpiresAt:     time.Now().Add(duration),
	}

	storageKey := storageKey(meta.PublicID)
	if err := h.fileStore.Put(r.Context(), storageKey, file, header.Size); err != nil {
		return apperrors.InternalError("failed to upload blob to S3", err)
	}

	if err := h.repo.Create(r.Context(), secret); err != nil {
		_ = h.fileStore.Delete(r.Context(), storageKey)
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("secret with this public_id already exists")
		}
		return apperrors.InternalError("failed to create secret", err)
	}

	if h.metrics != nil {
		h.metrics.SecretsCreated.Inc()
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"expires_at": secret.ExpiresAt.UTC().Format(time.RFC3339),
	})
	return nil
}

func (h *SecretHandler) RetrieveSecret(w http.ResponseWriter, r *http.Request) error {
	publicID := chi.URLParam(r, "publicID")
	if publicID == "" {
		return apperrors.BadRequestError("missing public_id")
	}

	token := r.Header.Get("X-Retrieval-Token")
	if token == "" {
		return apperrors.BadRequestError("missing X-Retrieval-Token header")
	}

	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return apperrors.InternalError("failed to get secret", err)
	}

	if !crypto.TokensEqual(token, secret.RetrievalToken) {
		return apperrors.ForbiddenError("invalid retrieval token")
	}

	obj, err := h.fileStore.Get(r.Context(), storageKey(publicID))
	if err != nil {
		return apperrors.InternalError("failed to get blob from S3", err)
	}
	defer func() { _ = obj.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(secret.BlobSize, 10))
	w.Header().Set("X-Burn-After-Read", fmt.Sprintf("%t", secret.BurnAfterRead))

	if _, err := io.Copy(w, obj); err != nil {
		slog.ErrorContext(r.Context(), "failed to stream blob to client", "error", err)
		return nil
	}

	if h.metrics != nil {
		h.metrics.SecretsRetrieved.Inc()
	}

	if secret.BurnAfterRead {
		if err := h.repo.Delete(r.Context(), publicID); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete burned secret", "error", err)
		}
		if err := h.fileStore.Delete(r.Context(), storageKey(publicID)); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete burned S3 object", "error", err)
		}
		if h.metrics != nil {
			h.metrics.SecretsDeleted.WithLabelValues("burn").Inc()
		}
	} else {
		if secret.RetrievedAt == nil {
			_ = h.repo.SetRetrievedAt(r.Context(), publicID)
		}
	}

	return nil
}

func (h *SecretHandler) SecretMetadata(w http.ResponseWriter, r *http.Request) error {
	publicID := chi.URLParam(r, "publicID")
	if publicID == "" {
		return apperrors.BadRequestError("missing public_id")
	}

	token := r.Header.Get("X-Retrieval-Token")
	if token == "" {
		return apperrors.BadRequestError("missing X-Retrieval-Token header")
	}

	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return apperrors.InternalError("failed to get secret", err)
	}

	if !crypto.TokensEqual(token, secret.RetrievalToken) {
		return apperrors.ForbiddenError("invalid retrieval token")
	}

	writeJSON(w, http.StatusOK, domain.SecretMetadataResponse{
		EncryptedMeta: secret.EncryptedMeta,
		BlobSize:      secret.BlobSize,
		BurnAfterRead: secret.BurnAfterRead,
		ExpiresAt:     secret.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:     secret.CreatedAt.UTC().Format(time.RFC3339),
	})
	return nil
}

func (h *SecretHandler) DeleteSecret(w http.ResponseWriter, r *http.Request) error {
	publicID := chi.URLParam(r, "publicID")
	if publicID == "" {
		return apperrors.BadRequestError("missing public_id")
	}

	retrievalToken := r.Header.Get("X-Retrieval-Token")
	if retrievalToken == "" {
		return apperrors.BadRequestError("missing X-Retrieval-Token header")
	}

	deletionToken := r.Header.Get("X-Deletion-Token")
	if deletionToken == "" {
		return apperrors.BadRequestError("missing X-Deletion-Token header")
	}

	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return apperrors.InternalError("failed to get secret", err)
	}

	if !crypto.TokensEqual(retrievalToken, secret.RetrievalToken) {
		return apperrors.ForbiddenError("invalid retrieval token")
	}

	if !crypto.TokensEqual(deletionToken, secret.DeletionToken) {
		return apperrors.ForbiddenError("invalid deletion token")
	}

	if err := h.fileStore.Delete(r.Context(), storageKey(publicID)); err != nil {
		_ = err
	}

	if err := h.repo.Delete(r.Context(), publicID); err != nil {
		return apperrors.InternalError("failed to delete secret", err)
	}

	if h.metrics != nil {
		h.metrics.SecretsDeleted.WithLabelValues("api").Inc()
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

var expirationDurations = map[string]time.Duration{
	"5m":  5 * time.Minute,
	"10m": 10 * time.Minute,
	"15m": 15 * time.Minute,
	"1h":  1 * time.Hour,
	"4h":  4 * time.Hour,
	"12h": 12 * time.Hour,
	"1d":  24 * time.Hour,
	"3d":  72 * time.Hour,
	"7d":  168 * time.Hour,
}

func parseExpiration(s string) (time.Duration, error) {
	d, ok := expirationDurations[s]
	if !ok {
		return 0, errors.New("invalid expiration: must be one of 5m, 10m, 15m, 1h, 4h, 12h, 1d, 3d, 7d")
	}
	return d, nil
}

func storageKey(publicID string) string {
	return "secrets/" + publicID
}
