package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pscheid92/secretli/internal/crypto"
	"github.com/pscheid92/secretli/internal/metrics"
	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/storage"
	"github.com/pscheid92/secretli/internal/store"
)

type FileHandler struct {
	repo        store.SecretRepo
	fileStore   storage.FileStore
	MaxFileSize int64
	metrics     *metrics.SecretMetrics
}

func NewFileHandler(repo store.SecretRepo, fileStore storage.FileStore, maxFileSize int64, m *metrics.SecretMetrics) *FileHandler {
	return &FileHandler{repo: repo, fileStore: fileStore, MaxFileSize: maxFileSize, metrics: m}
}

func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	// Enforce max file size on the entire request body
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxFileSize+1<<20) // extra 1MB for metadata

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusBadRequest, "file exceeds maximum size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	// Parse metadata JSON part
	metadataStr := r.FormValue("metadata")
	if metadataStr == "" {
		writeError(w, http.StatusBadRequest, "missing metadata part")
		return
	}

	var meta model.CreateFileRequest
	if err := json.Unmarshal([]byte(metadataStr), &meta); err != nil {
		writeError(w, http.StatusBadRequest, "invalid metadata JSON")
		return
	}

	if details := validateRequest(&meta); details != nil {
		writeValidationError(w, details)
		return
	}

	duration, err := parseExpiration(meta.Expiration)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get file part
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file part")
		return
	}
	defer func() { _ = file.Close() }()

	// Stream file to S3
	storageKey := fmt.Sprintf("secrets/%s", meta.PublicID)
	if err := h.fileStore.Put(r.Context(), storageKey, file, header.Size); err != nil {
		slog.Error("failed to upload file to S3", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Create database record
	expiresAt := time.Now().Add(duration)
	encryptedSize := header.Size

	secret := &model.Secret{
		PublicID:           meta.PublicID,
		RetrievalTokenHash: crypto.HashToken(meta.RetrievalToken),
		DeletionTokenHash:  crypto.HashToken(meta.DeletionToken),
		Nonce:              meta.Nonce,
		SecretType:         "file",
		StorageKey:         &storageKey,
		EncryptedFilename:  &meta.EncryptedFilename,
		EncryptedSize:      &encryptedSize,
		BurnAfterRead:      meta.BurnAfterRead,
		PasswordProtected:  meta.PasswordProtected,
		ExpiresAt:          expiresAt,
	}

	if err := h.repo.Create(r.Context(), secret); err != nil {
		// Clean up S3 object on DB error
		_ = h.fileStore.Delete(r.Context(), storageKey)
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "secret with this public_id already exists")
			return
		}
		slog.Error("failed to create file secret", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if h.metrics != nil {
		h.metrics.SecretsCreated.WithLabelValues("file").Inc()
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "publicID")
	if publicID == "" {
		writeError(w, http.StatusBadRequest, "missing public_id")
		return
	}

	token := r.Header.Get("X-Retrieval-Token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing X-Retrieval-Token header")
		return
	}

	// Fetch the secret
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

	// Ensure this is a file secret
	if secret.SecretType != "file" {
		writeError(w, http.StatusBadRequest, "secret is not a file type")
		return
	}

	if secret.StorageKey == nil {
		slog.Error("file secret missing storage key", "public_id", publicID)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Stream S3 object to response
	obj, err := h.fileStore.Get(r.Context(), *secret.StorageKey)
	if err != nil {
		slog.Error("failed to get file from S3", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = obj.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	if secret.EncryptedFilename != nil {
		w.Header().Set("X-Encrypted-Filename", *secret.EncryptedFilename)
	}
	w.Header().Set("X-Burn-After-Read", fmt.Sprintf("%t", secret.BurnAfterRead))
	w.Header().Set("X-Password-Protected", fmt.Sprintf("%t", secret.PasswordProtected))
	w.Header().Set("X-Nonce", secret.Nonce)

	if _, err := io.Copy(w, obj); err != nil {
		slog.Error("failed to stream file to client", "error", err)
		return
	}

	if h.metrics != nil {
		h.metrics.SecretsRetrieved.Inc()
	}

	// Handle burn-after-read: delete after streaming
	if secret.BurnAfterRead {
		if err := h.repo.Delete(r.Context(), publicID); err != nil {
			slog.Error("failed to delete burned secret", "error", err)
		}
		if err := h.fileStore.Delete(r.Context(), *secret.StorageKey); err != nil {
			slog.Error("failed to delete burned S3 object", "error", err)
		}
		if h.metrics != nil {
			h.metrics.SecretsDeleted.WithLabelValues("burn").Inc()
		}
	} else {
		if secret.RetrievedAt == nil {
			_ = h.repo.SetRetrievedAt(r.Context(), publicID)
		}
	}
}
