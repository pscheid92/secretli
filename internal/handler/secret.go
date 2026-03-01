package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/pscheid92/secretli/internal/crypto"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
)

const maxEncryptedDataSize = 1 << 20 // 1MB

type SecretHandler struct {
	repo           store.SecretRepo
	fileStore      storage.FileStore
	userSecretRepo store.UserSecretRepo
}

func NewSecretHandler(repo store.SecretRepo, fileStore storage.FileStore, userSecretRepo store.UserSecretRepo) *SecretHandler {
	return &SecretHandler{repo: repo, fileStore: fileStore, userSecretRepo: userSecretRepo}
}

func (h *SecretHandler) CreateSecret(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate required fields
	if req.PublicID == "" || req.RetrievalToken == "" || req.DeletionToken == "" || req.Nonce == "" || req.EncryptedData == "" {
		writeError(w, http.StatusBadRequest, "missing required fields")
		return
	}

	// Validate encrypted data size
	if len(req.EncryptedData) > maxEncryptedDataSize {
		writeError(w, http.StatusBadRequest, "encrypted_data exceeds 1MB limit")
		return
	}

	// Parse expiration
	duration, err := parseExpiration(req.Expiration)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	expiresAt := time.Now().Add(duration)

	secret := &model.Secret{
		PublicID:           req.PublicID,
		RetrievalTokenHash: crypto.HashToken(req.RetrievalToken),
		DeletionTokenHash:  crypto.HashToken(req.DeletionToken),
		EncryptedData:      &req.EncryptedData,
		Nonce:              req.Nonce,
		SecretType:         "text",
		BurnAfterRead:      req.BurnAfterRead,
		PasswordProtected:  req.PasswordProtected,
		ExpiresAt:          expiresAt,
	}

	if err := h.repo.Create(r.Context(), secret); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "secret with this public_id already exists")
			return
		}
		slog.Error("failed to create secret", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Link secret to authenticated user if present
	if user := UserFromContext(r.Context()); user != nil && h.userSecretRepo != nil {
		if err := h.userSecretRepo.LinkSecret(r.Context(), user.ID, secret.ID, req.Label); err != nil {
			slog.Error("failed to link secret to user", "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *SecretHandler) RetrieveSecret(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("publicID")
	if publicID == "" {
		writeError(w, http.StatusBadRequest, "missing public_id")
		return
	}

	token := r.Header.Get("X-Retrieval-Token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing X-Retrieval-Token header")
		return
	}

	// First fetch the secret to verify the token
	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	if err != nil {
		slog.Error("failed to get secret", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Verify retrieval token
	if !crypto.VerifyToken(token, secret.RetrievalTokenHash) {
		writeError(w, http.StatusForbidden, "invalid retrieval token")
		return
	}

	// Handle burn-after-read atomically
	if secret.BurnAfterRead {
		secret, err = h.repo.GetAndDeleteByPublicID(r.Context(), publicID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "secret not found")
			return
		}
		if err != nil {
			slog.Error("failed to burn secret", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	} else {
		// Set retrieved_at on first retrieval
		if secret.RetrievedAt == nil {
			_ = h.repo.SetRetrievedAt(r.Context(), publicID)
		}
	}

	type retrieveResponse struct {
		Nonce             string `json:"nonce"`
		EncryptedData     string `json:"encrypted_data"`
		SecretType        string `json:"secret_type"`
		BurnAfterRead     bool   `json:"burn_after_read"`
		PasswordProtected bool   `json:"password_protected"`
	}

	encData := ""
	if secret.EncryptedData != nil {
		encData = *secret.EncryptedData
	}

	writeJSON(w, http.StatusOK, retrieveResponse{
		Nonce:             secret.Nonce,
		EncryptedData:     encData,
		SecretType:        secret.SecretType,
		BurnAfterRead:     secret.BurnAfterRead,
		PasswordProtected: secret.PasswordProtected,
	})
}

func (h *SecretHandler) SecretMetadata(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("publicID")
	if publicID == "" {
		writeError(w, http.StatusBadRequest, "missing public_id")
		return
	}

	token := r.Header.Get("X-Retrieval-Token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing X-Retrieval-Token header")
		return
	}

	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	if err != nil {
		slog.Error("failed to get secret", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if !crypto.VerifyToken(token, secret.RetrievalTokenHash) {
		writeError(w, http.StatusForbidden, "invalid retrieval token")
		return
	}

	type metadataResponse struct {
		SecretType        string `json:"secret_type"`
		BurnAfterRead     bool   `json:"burn_after_read"`
		PasswordProtected bool   `json:"password_protected"`
		ExpiresAt         string `json:"expires_at"`
		CreatedAt         string `json:"created_at"`
	}

	writeJSON(w, http.StatusOK, metadataResponse{
		SecretType:        secret.SecretType,
		BurnAfterRead:     secret.BurnAfterRead,
		PasswordProtected: secret.PasswordProtected,
		ExpiresAt:         secret.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:         secret.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *SecretHandler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("publicID")
	if publicID == "" {
		writeError(w, http.StatusBadRequest, "missing public_id")
		return
	}

	retrievalToken := r.Header.Get("X-Retrieval-Token")
	if retrievalToken == "" {
		writeError(w, http.StatusBadRequest, "missing X-Retrieval-Token header")
		return
	}

	deletionToken := r.Header.Get("X-Deletion-Token")
	if deletionToken == "" {
		writeError(w, http.StatusBadRequest, "missing X-Deletion-Token header")
		return
	}

	// Fetch secret to verify tokens
	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "secret not found")
		return
	}
	if err != nil {
		slog.Error("failed to get secret", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if !crypto.VerifyToken(retrievalToken, secret.RetrievalTokenHash) {
		writeError(w, http.StatusForbidden, "invalid retrieval token")
		return
	}

	if !crypto.VerifyToken(deletionToken, secret.DeletionTokenHash) {
		writeError(w, http.StatusForbidden, "invalid deletion token")
		return
	}

	// Delete S3 object for file secrets before DB deletion
	if secret.SecretType == "file" && secret.StorageKey != nil && h.fileStore != nil {
		if err := h.fileStore.Delete(r.Context(), *secret.StorageKey); err != nil {
			slog.Error("failed to delete S3 object", "error", err, "key", *secret.StorageKey)
		}
	}

	if err := h.repo.Delete(r.Context(), publicID); err != nil {
		slog.Error("failed to delete secret", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
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
