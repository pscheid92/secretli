package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

type FileHandler struct {
	repo        domain.SecretRepo
	fileStore   domain.FileStore
	MaxFileSize int64
	metrics     *metrics.SecretMetrics
}

func NewFileHandler(repo domain.SecretRepo, fileStore domain.FileStore, maxFileSize int64, m *metrics.SecretMetrics) *FileHandler {
	return &FileHandler{repo: repo, fileStore: fileStore, MaxFileSize: maxFileSize, metrics: m}
}

func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) error {
	// Enforce max file size on the entire request body
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxFileSize+1<<20) // extra 1MB for metadata

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return apperrors.BadRequestError("file exceeds maximum size limit")
		}
		return apperrors.BadRequestError("invalid multipart form")
	}

	// Parse metadata JSON part
	metadataStr := r.FormValue("metadata")
	if metadataStr == "" {
		return apperrors.BadRequestError("missing metadata part")
	}

	var meta domain.CreateFileRequest
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

	// Get file part
	file, header, err := r.FormFile("file")
	if err != nil {
		return apperrors.BadRequestError("missing file part")
	}
	defer func() { _ = file.Close() }()

	// Stream file to S3
	storageKey := fmt.Sprintf("secrets/%s", meta.PublicID)
	if err := h.fileStore.Put(r.Context(), storageKey, file, header.Size); err != nil {
		return apperrors.InternalError("failed to upload file to S3", err)
	}

	// Create database record
	expiresAt := time.Now().Add(duration)
	encryptedSize := header.Size

	secret := &domain.Secret{
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
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("secret with this public_id already exists")
		}
		return apperrors.InternalError("failed to create file secret", err)
	}

	if h.metrics != nil {
		h.metrics.SecretsCreated.WithLabelValues("file").Inc()
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
	return nil
}

func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) error {
	publicID := chi.URLParam(r, "publicID")
	if publicID == "" {
		return apperrors.BadRequestError("missing public_id")
	}

	token := r.Header.Get("X-Retrieval-Token")
	if token == "" {
		return apperrors.BadRequestError("missing X-Retrieval-Token header")
	}

	// Fetch the secret
	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return apperrors.InternalError("failed to get secret", err)
	}

	// Verify retrieval token
	if !crypto.VerifyToken(token, secret.RetrievalTokenHash) {
		return apperrors.ForbiddenError("invalid retrieval token")
	}

	// Ensure this is a file secret
	if secret.SecretType != "file" {
		return apperrors.BadRequestError("secret is not a file type")
	}

	if secret.StorageKey == nil {
		return apperrors.InternalError("file secret missing storage key", nil)
	}

	// Stream S3 object to response
	obj, err := h.fileStore.Get(r.Context(), *secret.StorageKey)
	if err != nil {
		return apperrors.InternalError("failed to get file from S3", err)
	}
	defer func() { _ = obj.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	if secret.EncryptedFilename != nil {
		w.Header().Set("X-Encrypted-Filename", *secret.EncryptedFilename)
	}
	w.Header().Set("X-Burn-After-Read", fmt.Sprintf("%t", secret.BurnAfterRead))
	w.Header().Set("X-Password-Protected", fmt.Sprintf("%t", secret.PasswordProtected))
	w.Header().Set("X-Nonce", secret.Nonce)

	// After headers are written and streaming begins, we can't return errors
	// to the client, so log and return nil.
	if _, err := io.Copy(w, obj); err != nil {
		slog.ErrorContext(r.Context(), "failed to stream file to client", "error", err)
		return nil
	}

	if h.metrics != nil {
		h.metrics.SecretsRetrieved.Inc()
	}

	// Handle burn-after-read: delete after streaming
	if secret.BurnAfterRead {
		if err := h.repo.Delete(r.Context(), publicID); err != nil {
			slog.ErrorContext(r.Context(), "failed to delete burned secret", "error", err)
		}
		if err := h.fileStore.Delete(r.Context(), *secret.StorageKey); err != nil {
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
