package httpserver

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

const (
	HeaderRetrievalToken = "X-Retrieval-Token"
	HeaderDeletionToken  = "X-Deletion-Token"
	HeaderBurnAfterRead  = "X-Burn-After-Read"
)

type SecretHandler struct {
	repo        domain.SecretRepo
	fileStore   domain.FileStore
	maxFileSize int64
	metrics     *metrics.SecretMetrics
	validate    *validator.Validate
}

func NewSecretHandler(repo domain.SecretRepo, fileStore domain.FileStore, maxFileSize int64, m *metrics.SecretMetrics) *SecretHandler {
	return &SecretHandler{repo: repo, fileStore: fileStore, maxFileSize: maxFileSize, metrics: m, validate: newValidator()}
}

func (h *SecretHandler) CreateSecret(c echo.Context) error {
	r := c.Request()
	ctx := r.Context()
	r.Body = http.MaxBytesReader(c.Response(), r.Body, h.maxFileSize+1<<20)

	err := r.ParseMultipartForm(1 << 20)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return apperrors.BadRequestError("request exceeds maximum size limit")
	}
	if err != nil {
		return apperrors.BadRequestError("invalid multipart form")
	}

	var meta domain.CreateSecretRequest
	if err := c.Bind(&meta); err != nil {
		return apperrors.BadRequestError("invalid form data")
	}

	if details := h.validateRequest(&meta); details != nil {
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
		PublicID:       meta.PublicID,
		RetrievalToken: meta.RetrievalToken,
		DeletionToken:  meta.DeletionToken,
		EncryptedMeta:  meta.EncryptedMeta,
		BlobSize:       header.Size,
		BurnAfterRead:  meta.BurnAfterRead,
		ExpiresAt:      time.Now().Add(duration),
	}

	sk := storageKey(meta.PublicID)
	if err := h.fileStore.Put(ctx, sk, file, header.Size); err != nil {
		return apperrors.InternalError("failed to upload blob to S3", err)
	}

	if err := h.repo.Create(ctx, secret); err != nil {
		_ = h.fileStore.Delete(ctx, sk)
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("secret with this public_id already exists")
		}
		return apperrors.InternalError("failed to create secret", err)
	}

	h.metrics.SecretsCreated.Inc()

	response := map[string]string{
		"expires_at": secret.ExpiresAt.UTC().Format(time.RFC3339),
	}
	return c.JSON(http.StatusCreated, response)
}

func (h *SecretHandler) RetrieveSecret(c echo.Context) error {
	secret, err := h.authenticateSecret(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	publicID := c.Param("publicID")
	sk := storageKey(publicID)

	obj, err := h.fileStore.Get(ctx, sk)
	if err != nil {
		return apperrors.InternalError("failed to get blob from S3", err)
	}
	defer func() { _ = obj.Close() }()

	if secret.BurnAfterRead {
		token := c.Request().Header.Get(HeaderRetrievalToken)
		if err := h.repo.ClaimBurnAfterRead(ctx, publicID, token); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return apperrors.NotFoundError("secret not found")
			}
			return apperrors.InternalError("failed to claim burn-after-read secret", err)
		}
	}

	resp := c.Response()
	resp.Header().Set("Content-Type", "application/octet-stream")
	resp.Header().Set("Content-Length", strconv.FormatInt(secret.BlobSize, 10))
	resp.Header().Set(HeaderBurnAfterRead, fmt.Sprintf("%t", secret.BurnAfterRead))
	resp.WriteHeader(http.StatusOK)

	if _, err := io.Copy(resp, obj); err != nil {
		slog.ErrorContext(ctx, "failed to stream blob to client", "error", err)
		return nil
	}

	h.metrics.SecretsRetrieved.Inc()

	return nil
}

func (h *SecretHandler) SecretMetadata(c echo.Context) error {
	secret, err := h.authenticateSecret(c)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, domain.SecretMetadataResponse{
		EncryptedMeta: secret.EncryptedMeta,
		BlobSize:      secret.BlobSize,
		BurnAfterRead: secret.BurnAfterRead,
		ExpiresAt:     secret.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:     secret.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *SecretHandler) DeleteSecret(c echo.Context) error {
	secret, err := h.authenticateSecret(c)
	if err != nil {
		return err
	}

	r := c.Request()
	ctx := r.Context()

	deletionToken := r.Header.Get(HeaderDeletionToken)
	if deletionToken == "" {
		return apperrors.BadRequestError("missing " + HeaderDeletionToken + " header")
	}

	if !crypto.TokensEqual(deletionToken, secret.DeletionToken) {
		return apperrors.ForbiddenError("invalid deletion token")
	}

	publicID := c.Param("publicID")

	sk := storageKey(publicID)
	_ = h.fileStore.Delete(ctx, sk)

	if err := h.repo.Delete(ctx, publicID); err != nil {
		return apperrors.InternalError("failed to delete secret", err)
	}

	h.metrics.SecretsDeleted.WithLabelValues("api").Inc()

	return c.NoContent(http.StatusNoContent)
}

func (h *SecretHandler) authenticateSecret(c echo.Context) (*domain.Secret, error) {
	publicID := c.Param("publicID")
	if publicID == "" {
		return nil, apperrors.BadRequestError("missing public_id")
	}

	r := c.Request()
	token := r.Header.Get(HeaderRetrievalToken)
	if token == "" {
		return nil, apperrors.BadRequestError("missing " + HeaderRetrievalToken + " header")
	}

	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return nil, apperrors.InternalError("failed to get secret", err)
	}

	if !crypto.TokensEqual(token, secret.RetrievalToken) {
		return nil, apperrors.ForbiddenError("invalid retrieval token")
	}

	if secret.BurnAfterRead && secret.RetrievedAt != nil {
		return nil, apperrors.NotFoundError("secret not found")
	}

	return secret, nil
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
