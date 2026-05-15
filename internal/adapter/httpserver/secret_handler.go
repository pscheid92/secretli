package httpserver

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

const (
	HeaderMetadataToken = "X-Metadata-Token"
	HeaderBlobToken     = "X-Blob-Token"
	HeaderDeletionToken = "X-Deletion-Token"
	HeaderBurnAfterRead = "X-Burn-After-Read"

	retrievalSessionTTL = 15 * time.Minute
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

	if header.Size > h.maxFileSize {
		return apperrors.BadRequestError("file exceeds maximum size limit")
	}

	now := time.Now()
	secret := &domain.Secret{
		PublicID:          meta.PublicID,
		MetadataTokenHash: crypto.TokenHash(meta.MetadataToken),
		BlobTokenHash:     crypto.TokenHash(meta.BlobToken),
		DeletionTokenHash: crypto.TokenHash(meta.DeletionToken),
		EncryptedMeta:     meta.EncryptedMeta,
		BlobSize:          header.Size,
		BurnAfterRead:     meta.BurnAfterRead,
		ExpiresAt:         now.Add(duration),
		CreatedAt:         now,
	}

	sk := domain.SecretStorageKey(meta.PublicID)
	if err := h.fileStore.Put(ctx, sk, file, header.Size); err != nil {
		return apperrors.InternalError("failed to upload blob to S3", err)
	}

	if err := h.repo.Create(ctx, secret, now); err != nil {
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
	secret, err := h.authenticateBlob(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	publicID := c.Param("publicID")
	sk := domain.SecretStorageKey(publicID)

	obj, err := h.fileStore.Get(ctx, sk)
	if err != nil {
		return apperrors.InternalError("failed to get blob from S3", err)
	}
	defer func() { _ = obj.Close() }()

	if secret.BurnAfterRead {
		token := c.Request().Header.Get(HeaderBlobToken)
		if err := h.repo.ClaimBurnAfterRead(ctx, publicID, crypto.TokenHash(token), time.Now()); err != nil {
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

func (h *SecretHandler) StartRetrievalSession(c echo.Context) error {
	publicID := c.Param("publicID")
	if publicID == "" {
		return apperrors.BadRequestError("missing public_id")
	}
	if !domain.ValidPublicID(publicID) {
		return apperrors.BadRequestError("malformed public_id")
	}

	token := c.Request().Header.Get(HeaderBlobToken)
	if token == "" {
		return apperrors.BadRequestError("missing " + HeaderBlobToken + " header")
	}
	if !domain.ValidToken(token) {
		return apperrors.BadRequestError("malformed " + HeaderBlobToken + " header")
	}

	sessionToken, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create retrieval session", err)
	}
	now := time.Now()
	sessionExpiresAt := now.Add(retrievalSessionTTL)

	secret, err := h.repo.StartRetrievalSession(
		c.Request().Context(),
		publicID,
		crypto.TokenHash(token),
		crypto.TokenHash(sessionToken),
		sessionExpiresAt,
		now,
	)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("secret not found")
	}
	if errors.Is(err, domain.ErrForbidden) {
		return apperrors.ForbiddenError("invalid blob token")
	}
	if err != nil {
		return apperrors.InternalError("failed to start retrieval session", err)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"session_token":   sessionToken,
		"blob_size":       secret.BlobSize,
		"expires_at":      sessionExpiresAt.UTC().Format(time.RFC3339),
		"burn_after_read": secret.BurnAfterRead,
	})
}

func (h *SecretHandler) RetrieveSecretRange(c echo.Context) error {
	publicID := c.Param("publicID")
	if publicID == "" {
		return apperrors.BadRequestError("missing public_id")
	}
	if !domain.ValidPublicID(publicID) {
		return apperrors.BadRequestError("malformed public_id")
	}

	sessionToken, err := bearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}
	if !domain.ValidToken(sessionToken) {
		return apperrors.BadRequestError("malformed Authorization header")
	}

	secret, err := h.repo.GetByRetrievalSession(
		c.Request().Context(),
		publicID,
		crypto.TokenHash(sessionToken),
		time.Now(),
	)
	if errors.Is(err, domain.ErrForbidden) {
		return apperrors.ForbiddenError("invalid retrieval session")
	}
	if err != nil {
		return apperrors.InternalError("failed to validate retrieval session", err)
	}

	start, end, err := parseBoundedRange(c.Request().Header.Get("Range"), secret.BlobSize)
	if errors.Is(err, errRangeOutOfBounds) {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", secret.BlobSize))
		return c.NoContent(http.StatusRequestedRangeNotSatisfiable)
	}
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}

	ctx := c.Request().Context()
	obj, err := h.fileStore.GetRange(ctx, domain.SecretStorageKey(publicID), start, end)
	if err != nil {
		return apperrors.InternalError("failed to get blob range from S3", err)
	}
	defer func() { _ = obj.Close() }()

	contentLength := end - start + 1
	resp := c.Response()
	resp.Header().Set("Accept-Ranges", "bytes")
	resp.Header().Set("Content-Type", "application/octet-stream")
	resp.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	resp.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, secret.BlobSize))
	resp.WriteHeader(http.StatusPartialContent)

	if _, err := io.Copy(resp, obj); err != nil {
		slog.ErrorContext(ctx, "failed to stream blob range to client", "error", err)
		return nil
	}

	return nil
}

func (h *SecretHandler) SecretMetadata(c echo.Context) error {
	secret, err := h.authenticateMetadata(c)
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
	secret, err := h.authenticateMetadata(c)
	if err != nil {
		return err
	}

	r := c.Request()
	ctx := r.Context()

	deletionToken := r.Header.Get(HeaderDeletionToken)
	if deletionToken == "" {
		return apperrors.BadRequestError("missing " + HeaderDeletionToken + " header")
	}
	if !domain.ValidToken(deletionToken) {
		return apperrors.BadRequestError("malformed " + HeaderDeletionToken + " header")
	}

	if !crypto.TokensEqual(crypto.TokenHash(deletionToken), secret.DeletionTokenHash) {
		return apperrors.ForbiddenError("invalid deletion token")
	}

	publicID := c.Param("publicID")

	sk := domain.SecretStorageKey(publicID)
	if err := h.fileStore.Delete(ctx, sk); err != nil {
		return apperrors.InternalError("failed to delete blob from S3", err)
	}

	if err := h.repo.Delete(ctx, publicID); err != nil {
		return apperrors.InternalError("failed to delete secret", err)
	}

	h.metrics.SecretsDeleted.WithLabelValues("api").Inc()

	return c.NoContent(http.StatusNoContent)
}

func (h *SecretHandler) authenticateMetadata(c echo.Context) (*domain.Secret, error) {
	return h.authenticateSecret(c, HeaderMetadataToken, func(secret *domain.Secret) string {
		return secret.MetadataTokenHash
	})
}

func (h *SecretHandler) authenticateBlob(c echo.Context) (*domain.Secret, error) {
	return h.authenticateSecret(c, HeaderBlobToken, func(secret *domain.Secret) string {
		return secret.BlobTokenHash
	})
}

func (h *SecretHandler) authenticateSecret(c echo.Context, header string, expectedHash func(*domain.Secret) string) (*domain.Secret, error) {
	publicID := c.Param("publicID")
	if publicID == "" {
		return nil, apperrors.BadRequestError("missing public_id")
	}
	if !domain.ValidPublicID(publicID) {
		return nil, apperrors.BadRequestError("malformed public_id")
	}

	r := c.Request()
	token := r.Header.Get(header)
	if token == "" {
		return nil, apperrors.BadRequestError("missing " + header + " header")
	}
	if !domain.ValidToken(token) {
		return nil, apperrors.BadRequestError("malformed " + header + " header")
	}

	secret, err := h.repo.GetByPublicID(r.Context(), publicID, time.Now())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return nil, apperrors.InternalError("failed to get secret", err)
	}

	if !crypto.TokensEqual(crypto.TokenHash(token), expectedHash(secret)) {
		return nil, apperrors.ForbiddenError("invalid token")
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

func newRetrievalSessionToken() (string, error) {
	token := make([]byte, 32)
	if _, err := cryptorand.Read(token); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func bearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("missing Authorization header")
	}
	prefix, token, ok := strings.Cut(header, " ")
	if !ok || prefix != "Bearer" || token == "" || strings.Contains(token, " ") {
		return "", errors.New("malformed Authorization header")
	}
	return token, nil
}

var errRangeOutOfBounds = errors.New("range out of bounds")

func parseBoundedRange(header string, size int64) (int64, int64, error) {
	if header == "" {
		return 0, 0, errors.New("missing Range header")
	}
	unit, span, ok := strings.Cut(header, "=")
	if !ok || unit != "bytes" || strings.Contains(span, ",") {
		return 0, 0, errors.New("malformed Range header")
	}
	startText, endText, ok := strings.Cut(span, "-")
	if !ok || startText == "" || endText == "" {
		return 0, 0, errors.New("malformed Range header")
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, errors.New("malformed Range header")
	}
	end, err := strconv.ParseInt(endText, 10, 64)
	if err != nil || end < start {
		return 0, 0, errors.New("malformed Range header")
	}
	if size <= 0 || start >= size || end >= size {
		return 0, 0, errRangeOutOfBounds
	}
	return start, end, nil
}
