package httpserver

import (
	"context"
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

	"github.com/labstack/echo/v4"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

const (
	// Header names, not credentials.
	HeaderMetadataToken = "X-Metadata-Token" //nolint:gosec
	HeaderBlobToken     = "X-Blob-Token"     //nolint:gosec
	HeaderDeletionToken = "X-Deletion-Token" //nolint:gosec

	retrievalSessionTTL = 15 * time.Minute
	// maxRangeBytes caps a single range request. The client coalesces at most
	// 64 MiB of plaintext per request; without a cap one session could pull
	// the full blob hundreds of times per minute.
	maxRangeBytes = 128 * 1024 * 1024
)

// SecretHandler serves everything a client does with an existing secret:
// metadata, retrieval sessions, range reads and deletion. Secrets are created
// through UploadHandler's multipart upload sessions.
type SecretHandler struct {
	repo      domain.SecretRepo
	fileStore domain.FileStore
	metrics   *metrics.SecretMetrics
}

func NewSecretHandler(repo domain.SecretRepo, fileStore domain.FileStore, m *metrics.SecretMetrics) *SecretHandler {
	return &SecretHandler{repo: repo, fileStore: fileStore, metrics: m}
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

	// The owner link carries the deletion token too. With it, the owner
	// opening their own secret is told apart from a recipient getting it.
	var deletionTokenHash string
	if deletionToken := c.Request().Header.Get(HeaderDeletionToken); deletionToken != "" {
		if !domain.ValidToken(deletionToken) {
			return apperrors.BadRequestError("malformed " + HeaderDeletionToken + " header")
		}
		deletionTokenHash = crypto.TokenHash(deletionToken)
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
		deletionTokenHash,
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
	obj, err := h.fileStore.GetRange(ctx, secret.StorageKey, start, end)
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
		OpenedAt:      rfc3339Pointer(secret.RetrievedAt),
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

	if err := h.fileStore.Delete(ctx, secret.StorageKey); err != nil {
		return apperrors.InternalError("failed to delete blob from S3", err)
	}

	// The row may already be gone if cleanup raced with this request; the
	// object is deleted either way, so report success.
	if err := h.repo.Delete(ctx, secret.PublicID, time.Now()); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return apperrors.InternalError("failed to delete secret", err)
	}

	h.metrics.SecretsDeleted.WithLabelValues("api").Inc()

	return c.NoContent(http.StatusNoContent)
}

func (h *SecretHandler) authenticateMetadata(c echo.Context) (*domain.Secret, error) {
	publicID := c.Param("publicID")
	if publicID == "" {
		return nil, apperrors.BadRequestError("missing public_id")
	}
	if !domain.ValidPublicID(publicID) {
		return nil, apperrors.BadRequestError("malformed public_id")
	}

	r := c.Request()
	token := r.Header.Get(HeaderMetadataToken)
	if token == "" {
		return nil, apperrors.BadRequestError("missing " + HeaderMetadataToken + " header")
	}
	if !domain.ValidToken(token) {
		return nil, apperrors.BadRequestError("malformed " + HeaderMetadataToken + " header")
	}
	tokenHash := crypto.TokenHash(token)
	now := time.Now()

	secret, err := h.repo.GetByPublicID(r.Context(), publicID, now)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, h.secretGone(r.Context(), publicID, tokenHash, now)
	}
	if err != nil {
		return nil, apperrors.InternalError("failed to get secret", err)
	}

	if !crypto.TokensEqual(tokenHash, secret.MetadataTokenHash) {
		return nil, apperrors.ForbiddenError("invalid token")
	}

	// A consumed one-time secret stays on file while its download runs, but
	// to everyone else it is gone.
	if secret.BurnAfterRead && secret.RetrievedAt != nil {
		return nil, h.secretGone(r.Context(), publicID, tokenHash, now)
	}

	return secret, nil
}

// secretGone answers for a secret that is not there any more: with what
// became of it, when the request carries the link's metadata token, and with
// a plain not found otherwise.
func (h *SecretHandler) secretGone(ctx context.Context, publicID, metadataTokenHash string, now time.Time) error {
	tomb, err := h.repo.GetTombstone(ctx, publicID, now)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("secret not found")
	}
	if err != nil {
		return apperrors.InternalError("failed to look up secret", err)
	}
	if !crypto.TokensEqual(metadataTokenHash, tomb.MetadataTokenHash) {
		return apperrors.NotFoundError("secret not found")
	}
	return apperrors.GoneError("secret is gone", tombstoneDetails(tomb))
}

// tombstoneDetails is what a client is told about a secret that is gone.
func tombstoneDetails(tomb *domain.SecretTombstone) map[string]any {
	details := map[string]any{
		"outcome":         string(tomb.Outcome),
		"burn_after_read": tomb.BurnAfterRead,
		"ended_at":        tomb.EndedAt.UTC().Format(time.RFC3339),
		"opened_by_owner": tomb.OpenedByOwner,
	}
	if tomb.FirstOpenedAt != nil {
		details["first_opened_at"] = tomb.FirstOpenedAt.UTC().Format(time.RFC3339)
	}
	return details
}

func rfc3339Pointer(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
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
	if end-start+1 > maxRangeBytes {
		return 0, 0, errRangeOutOfBounds
	}
	return start, end, nil
}
