package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

type SecretHandler struct {
	repo      domain.SecretRepo
	fileStore domain.FileStore
	metrics   *metrics.SecretMetrics
}

func NewSecretHandler(repo domain.SecretRepo, fileStore domain.FileStore, m *metrics.SecretMetrics) *SecretHandler {
	return &SecretHandler{repo: repo, fileStore: fileStore, metrics: m}
}

func (h *SecretHandler) CreateSecret(w http.ResponseWriter, r *http.Request) error {
	var req domain.CreateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apperrors.BadRequestError("invalid JSON body")
	}

	if details := validateRequest(&req); details != nil {
		return validationError(details)
	}

	duration, err := parseExpiration(req.Expiration)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}

	expiresAt := time.Now().Add(duration)

	secret := &domain.Secret{
		PublicID:           req.PublicID,
		RetrievalToken: req.RetrievalToken,
		DeletionToken:  req.DeletionToken,
		EncryptedData:      &req.EncryptedData,
		Nonce:              req.Nonce,
		SecretType:         "text",
		BurnAfterRead:      req.BurnAfterRead,
		PasswordProtected:  req.PasswordProtected,
		ExpiresAt:          expiresAt,
	}

	if err := h.repo.Create(r.Context(), secret); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("secret with this public_id already exists")
		}
		return apperrors.InternalError("failed to create secret", err)
	}

	if h.metrics != nil {
		h.metrics.SecretsCreated.WithLabelValues("text").Inc()
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
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

	if secret.BurnAfterRead {
		secret, err = h.repo.GetAndDeleteByPublicID(r.Context(), publicID)
		if errors.Is(err, domain.ErrNotFound) {
			return apperrors.NotFoundError("secret not found")
		}
		if err != nil {
			return apperrors.InternalError("failed to burn secret", err)
		}
		if h.metrics != nil {
			h.metrics.SecretsDeleted.WithLabelValues("burn").Inc()
		}
	} else {
		if secret.RetrievedAt == nil {
			_ = h.repo.SetRetrievedAt(r.Context(), publicID)
		}
	}

	if h.metrics != nil {
		h.metrics.SecretsRetrieved.Inc()
	}

	encData := ""
	if secret.EncryptedData != nil {
		encData = *secret.EncryptedData
	}

	writeJSON(w, http.StatusOK, domain.RetrieveSecretResponse{
		Nonce:             secret.Nonce,
		EncryptedData:     encData,
		SecretType:        secret.SecretType,
		BurnAfterRead:     secret.BurnAfterRead,
		PasswordProtected: secret.PasswordProtected,
	})
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
		SecretType:        secret.SecretType,
		BurnAfterRead:     secret.BurnAfterRead,
		PasswordProtected: secret.PasswordProtected,
		ExpiresAt:         secret.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:         secret.CreatedAt.UTC().Format(time.RFC3339),
		FileSize:          secret.EncryptedSize,
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

	// Delete S3 object for file secrets before DB deletion
	if secret.SecretType == "file" && secret.StorageKey != nil && h.fileStore != nil {
		if err := h.fileStore.Delete(r.Context(), *secret.StorageKey); err != nil {
			// Log but don't block deletion
			_ = err
		}
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
