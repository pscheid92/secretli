package httpserver

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	HeaderMetadataToken   = "X-Metadata-Token"
	HeaderBlobToken       = "X-Blob-Token"
	HeaderDeletionToken   = "X-Deletion-Token"
	HeaderBurnAfterRead   = "X-Burn-After-Read"
	HeaderEncryptedSHA256 = "X-Encrypted-SHA256"

	retrievalSessionTTL = 15 * time.Minute
	chunkedUploadTTL    = 24 * time.Hour
	maxManifestBodySize = 8 * 1024 * 1024
	chunkBodySlackBytes = 1 * 1024 * 1024
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

	secret := &domain.Secret{
		PublicID:          meta.PublicID,
		MetadataTokenHash: crypto.TokenHash(meta.MetadataToken),
		BlobTokenHash:     crypto.TokenHash(meta.BlobToken),
		DeletionTokenHash: crypto.TokenHash(meta.DeletionToken),
		EncryptedMeta:     meta.EncryptedMeta,
		BlobSize:          header.Size,
		BurnAfterRead:     meta.BurnAfterRead,
		ExpiresAt:         time.Now().Add(duration),
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

func (h *SecretHandler) CreateUpload(c echo.Context) error {
	r := c.Request()
	ctx := r.Context()
	r.Body = http.MaxBytesReader(c.Response(), r.Body, 1<<20)

	var req domain.CreateUploadRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid JSON body")
	}
	if details := h.validateRequest(&req); details != nil {
		return validationError(details)
	}
	if req.EncryptedTotalSize > h.maxFileSize {
		return apperrors.BadRequestError("encrypted upload exceeds maximum size limit")
	}

	duration, err := parseExpiration(req.Expiration)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}

	uploadToken, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create upload token", err)
	}
	uploadExpiresAt := time.Now().Add(chunkedUploadTTL)

	secret := &domain.Secret{
		PublicID:                  req.PublicID,
		MetadataTokenHash:         crypto.TokenHash(req.MetadataToken),
		BlobTokenHash:             crypto.TokenHash(req.BlobToken),
		DeletionTokenHash:         crypto.TokenHash(req.DeletionToken),
		EncryptedMeta:             req.EncryptedMeta,
		BlobSize:                  req.EncryptedTotalSize,
		BurnAfterRead:             req.BurnAfterRead,
		ExpiresAt:                 uploadExpiresAt,
		StorageVersion:            domain.StorageVersionChunked,
		Status:                    domain.SecretStatusPending,
		ExpirationDurationSeconds: int64(duration / time.Second),
		UploadTokenHash:           crypto.TokenHash(uploadToken),
		UploadExpiresAt:           &uploadExpiresAt,
		ChunkSize:                 req.ChunkSize,
		ChunkCount:                req.ChunkCount,
		EncryptedTotalSize:        req.EncryptedTotalSize,
	}

	if err := h.repo.CreateUpload(ctx, secret); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("secret with this public_id already exists")
		}
		return apperrors.InternalError("failed to create upload session", err)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"public_id":         req.PublicID,
		"upload_token":      uploadToken,
		"upload_expires_at": uploadExpiresAt.UTC().Format(time.RFC3339),
		"chunk_size":        req.ChunkSize,
	})
}

func (h *SecretHandler) UploadStatus(c echo.Context) error {
	secret, err := h.authenticateUpload(c)
	if err != nil {
		return err
	}
	objects, err := h.repo.ListObjects(c.Request().Context(), secret.PublicID)
	if err != nil {
		return apperrors.InternalError("failed to list uploaded chunks", err)
	}

	return c.JSON(http.StatusOK, uploadStatusResponse(secret, objects))
}

func (h *SecretHandler) UploadChunk(c echo.Context) error {
	secret, err := h.authenticateUpload(c)
	if err != nil {
		return err
	}

	index, err := parseChunkIndex(c.Param("index"))
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}
	if index < 0 || index >= secret.ChunkCount {
		return apperrors.BadRequestError("chunk index out of range")
	}

	return h.storeUploadObject(c, secret, domain.ObjectKindChunk, index, maxEncryptedChunkSize(secret, h.maxFileSize))
}

func (h *SecretHandler) UploadManifest(c echo.Context) error {
	secret, err := h.authenticateUpload(c)
	if err != nil {
		return err
	}
	return h.storeUploadObject(c, secret, domain.ObjectKindManifest, domain.ManifestObjectIndex, maxManifestBodySize)
}

func (h *SecretHandler) CompleteUpload(c echo.Context) error {
	secret, err := h.authenticateUpload(c)
	if err != nil {
		return err
	}

	objects, err := h.repo.ListObjects(c.Request().Context(), secret.PublicID)
	if err != nil {
		return apperrors.InternalError("failed to list uploaded chunks", err)
	}

	manifest, chunks := splitUploadObjects(objects)
	if manifest == nil {
		return apperrors.BadRequestError("missing encrypted manifest")
	}
	if int32(len(chunks)) != secret.ChunkCount {
		return apperrors.BadRequestError("missing encrypted chunk")
	}

	actualSize := manifest.EncryptedSize
	for i := int32(0); i < secret.ChunkCount; i++ {
		chunk, ok := chunks[i]
		if !ok {
			return apperrors.BadRequestError("missing encrypted chunk")
		}
		actualSize += chunk.EncryptedSize
	}
	if actualSize > h.maxFileSize {
		return apperrors.BadRequestError("encrypted upload exceeds maximum size limit")
	}

	expiresAt := time.Now().Add(time.Duration(secret.ExpirationDurationSeconds) * time.Second)
	if err := h.repo.CompleteUpload(c.Request().Context(), secret.PublicID, actualSize, expiresAt); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return apperrors.NotFoundError("upload session not found")
		}
		return apperrors.InternalError("failed to complete upload", err)
	}

	h.metrics.SecretsCreated.Inc()

	return c.JSON(http.StatusOK, map[string]string{
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *SecretHandler) CancelUpload(c echo.Context) error {
	secret, err := h.authenticateUpload(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	if err := h.deleteUploadObjects(ctx, secret); err != nil {
		return apperrors.InternalError("failed to delete uploaded objects", err)
	}
	if err := h.repo.Delete(ctx, secret.PublicID); err != nil {
		return apperrors.InternalError("failed to cancel upload", err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *SecretHandler) RetrieveSecret(c echo.Context) error {
	secret, err := h.authenticateBlob(c)
	if err != nil {
		return err
	}
	if secret.StorageVersion != "" && secret.StorageVersion != domain.StorageVersionSingle {
		return apperrors.BadRequestError("chunked secret must use chunk retrieval endpoints")
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
		token := c.Request().Header.Get(HeaderBlobToken)
		if err := h.repo.ClaimBurnAfterRead(ctx, publicID, crypto.TokenHash(token)); err != nil {
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
	sessionExpiresAt := time.Now().Add(retrievalSessionTTL)

	secret, err := h.repo.StartRetrievalSession(
		c.Request().Context(),
		publicID,
		crypto.TokenHash(token),
		crypto.TokenHash(sessionToken),
		sessionExpiresAt,
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
		"storage_version": secretStorageVersion(secret),
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
	)
	if errors.Is(err, domain.ErrForbidden) {
		return apperrors.ForbiddenError("invalid retrieval session")
	}
	if err != nil {
		return apperrors.InternalError("failed to validate retrieval session", err)
	}
	if secret.StorageVersion != "" && secret.StorageVersion != domain.StorageVersionSingle {
		return apperrors.BadRequestError("chunked secret must use chunk retrieval endpoints")
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
	obj, err := h.fileStore.GetRange(ctx, storageKey(publicID), start, end)
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

func (h *SecretHandler) RetrieveChunkedManifest(c echo.Context) error {
	secret, err := h.authenticateRetrievalSession(c)
	if err != nil {
		return err
	}
	if secret.StorageVersion != domain.StorageVersionChunked {
		return apperrors.BadRequestError("secret is not chunked")
	}

	return h.streamChunkedObject(c, secret.PublicID, domain.ObjectKindManifest, domain.ManifestObjectIndex)
}

func (h *SecretHandler) RetrieveChunkedChunk(c echo.Context) error {
	secret, err := h.authenticateRetrievalSession(c)
	if err != nil {
		return err
	}
	if secret.StorageVersion != domain.StorageVersionChunked {
		return apperrors.BadRequestError("secret is not chunked")
	}

	index, err := parseChunkIndex(c.Param("index"))
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}
	if index < 0 || index >= secret.ChunkCount {
		return apperrors.BadRequestError("chunk index out of range")
	}

	return h.streamChunkedObject(c, secret.PublicID, domain.ObjectKindChunk, index)
}

func (h *SecretHandler) SecretMetadata(c echo.Context) error {
	secret, err := h.authenticateMetadata(c)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, domain.SecretMetadataResponse{
		EncryptedMeta:  secret.EncryptedMeta,
		BlobSize:       secret.BlobSize,
		BurnAfterRead:  secret.BurnAfterRead,
		ExpiresAt:      secret.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:      secret.CreatedAt.UTC().Format(time.RFC3339),
		StorageVersion: secretStorageVersion(secret),
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

	if err := h.deleteStoredObjects(ctx, publicID); err != nil {
		return apperrors.InternalError("failed to delete blob from S3", err)
	}

	if err := h.repo.Delete(ctx, publicID); err != nil {
		return apperrors.InternalError("failed to delete secret", err)
	}

	h.metrics.SecretsDeleted.WithLabelValues("api").Inc()

	return c.NoContent(http.StatusNoContent)
}

func (h *SecretHandler) storeUploadObject(c echo.Context, secret *domain.Secret, objectKind string, objectIndex int32, maxBodySize int64) error {
	expectedHash := strings.ToLower(c.Request().Header.Get(HeaderEncryptedSHA256))
	if !validSHA256Hex(expectedHash) {
		return apperrors.BadRequestError("missing or malformed " + HeaderEncryptedSHA256 + " header")
	}
	if contentType := c.Request().Header.Get("Content-Type"); contentType != "" && contentType != "application/octet-stream" {
		return apperrors.BadRequestError("Content-Type must be application/octet-stream")
	}

	if existing, err := h.repo.GetObject(c.Request().Context(), secret.PublicID, objectKind, objectIndex); err == nil {
		if existing.SHA256Hex == expectedHash {
			return c.JSON(http.StatusOK, uploadObjectResponse(*existing))
		}
		return apperrors.ConflictError("object already uploaded with different content")
	} else if !errors.Is(err, domain.ErrNotFound) {
		return apperrors.InternalError("failed to check uploaded object", err)
	}

	r := c.Request()
	r.Body = http.MaxBytesReader(c.Response(), r.Body, maxBodySize)
	data, err := io.ReadAll(r.Body)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return apperrors.BadRequestError("object exceeds maximum size limit")
	}
	if err != nil {
		return apperrors.BadRequestError("failed to read encrypted object")
	}
	if len(data) == 0 {
		return apperrors.BadRequestError("encrypted object is empty")
	}

	actualHash := sha256Hex(data)
	key := chunkedObjectKey(secret.PublicID, objectKind, objectIndex)
	if actualHash != expectedHash {
		_ = h.fileStore.Delete(r.Context(), key)
		return apperrors.BadRequestError("encrypted object hash mismatch")
	}

	if err := h.fileStore.Put(r.Context(), key, bytes.NewReader(data), int64(len(data))); err != nil {
		return apperrors.InternalError("failed to store encrypted object", err)
	}

	object := domain.SecretObject{
		PublicID:      secret.PublicID,
		ObjectKind:    objectKind,
		ObjectIndex:   objectIndex,
		EncryptedSize: int64(len(data)),
		SHA256Hex:     actualHash,
	}
	if err := h.repo.CreateObject(r.Context(), &object); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			existing, getErr := h.repo.GetObject(r.Context(), secret.PublicID, objectKind, objectIndex)
			if getErr == nil && existing.SHA256Hex == actualHash && existing.EncryptedSize == int64(len(data)) {
				return c.JSON(http.StatusOK, uploadObjectResponse(*existing))
			}
			return apperrors.ConflictError("object already uploaded with different content")
		}
		_ = h.fileStore.Delete(r.Context(), key)
		return apperrors.InternalError("failed to record encrypted object", err)
	}

	return c.JSON(http.StatusCreated, uploadObjectResponse(object))
}

func (h *SecretHandler) streamChunkedObject(c echo.Context, publicID, objectKind string, objectIndex int32) error {
	object, err := h.repo.GetObject(c.Request().Context(), publicID, objectKind, objectIndex)
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.NotFoundError("encrypted object not found")
	}
	if err != nil {
		return apperrors.InternalError("failed to get encrypted object metadata", err)
	}

	obj, err := h.fileStore.Get(c.Request().Context(), chunkedObjectKey(publicID, objectKind, objectIndex))
	if err != nil {
		return apperrors.InternalError("failed to get encrypted object", err)
	}
	defer func() { _ = obj.Close() }()

	resp := c.Response()
	resp.Header().Set("Content-Type", "application/octet-stream")
	resp.Header().Set("Content-Length", strconv.FormatInt(object.EncryptedSize, 10))
	resp.Header().Set(HeaderEncryptedSHA256, object.SHA256Hex)
	resp.WriteHeader(http.StatusOK)

	if _, err := io.Copy(resp, obj); err != nil {
		slog.ErrorContext(c.Request().Context(), "failed to stream chunked object to client", "error", err)
		return nil
	}
	return nil
}

func (h *SecretHandler) deleteStoredObjects(ctx context.Context, publicID string) error {
	keys := map[string]struct{}{storageKey(publicID): {}}

	objects, err := h.repo.ListObjects(ctx, publicID)
	if err != nil {
		return err
	}
	for _, object := range objects {
		keys[chunkedObjectKey(object.PublicID, object.ObjectKind, object.ObjectIndex)] = struct{}{}
	}

	for key := range keys {
		if err := h.fileStore.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (h *SecretHandler) deleteUploadObjects(ctx context.Context, secret *domain.Secret) error {
	keys := map[string]struct{}{
		storageKey(secret.PublicID): {},
		chunkedObjectKey(secret.PublicID, domain.ObjectKindManifest, domain.ManifestObjectIndex): {},
	}
	for i := int32(0); i < secret.ChunkCount; i++ {
		keys[chunkedObjectKey(secret.PublicID, domain.ObjectKindChunk, i)] = struct{}{}
	}

	objects, err := h.repo.ListObjects(ctx, secret.PublicID)
	if err != nil {
		return err
	}
	for _, object := range objects {
		keys[chunkedObjectKey(object.PublicID, object.ObjectKind, object.ObjectIndex)] = struct{}{}
	}
	for key := range keys {
		if err := h.fileStore.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (h *SecretHandler) authenticateUpload(c echo.Context) (*domain.Secret, error) {
	publicID := c.Param("publicID")
	if publicID == "" {
		return nil, apperrors.BadRequestError("missing public_id")
	}
	if !domain.ValidPublicID(publicID) {
		return nil, apperrors.BadRequestError("malformed public_id")
	}

	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" {
		return nil, apperrors.ForbiddenError("invalid upload token")
	}
	token, err := bearerToken(authHeader)
	if err != nil {
		return nil, apperrors.BadRequestError(err.Error())
	}
	if !domain.ValidToken(token) {
		return nil, apperrors.BadRequestError("malformed Authorization header")
	}

	secret, err := h.repo.GetPendingUploadByPublicID(c.Request().Context(), publicID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperrors.NotFoundError("upload session not found")
	}
	if err != nil {
		return nil, apperrors.InternalError("failed to get upload session", err)
	}
	if !crypto.TokensEqual(crypto.TokenHash(token), secret.UploadTokenHash) {
		return nil, apperrors.ForbiddenError("invalid upload token")
	}
	return secret, nil
}

func (h *SecretHandler) authenticateRetrievalSession(c echo.Context) (*domain.Secret, error) {
	publicID := c.Param("publicID")
	if publicID == "" {
		return nil, apperrors.BadRequestError("missing public_id")
	}
	if !domain.ValidPublicID(publicID) {
		return nil, apperrors.BadRequestError("malformed public_id")
	}

	sessionToken, err := bearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return nil, apperrors.BadRequestError(err.Error())
	}
	if !domain.ValidToken(sessionToken) {
		return nil, apperrors.BadRequestError("malformed Authorization header")
	}

	secret, err := h.repo.GetByRetrievalSession(
		c.Request().Context(),
		publicID,
		crypto.TokenHash(sessionToken),
	)
	if errors.Is(err, domain.ErrForbidden) {
		return nil, apperrors.ForbiddenError("invalid retrieval session")
	}
	if err != nil {
		return nil, apperrors.InternalError("failed to validate retrieval session", err)
	}
	return secret, nil
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

	secret, err := h.repo.GetByPublicID(r.Context(), publicID)
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

type uploadObjectJSON struct {
	Index         int32  `json:"index"`
	EncryptedSize int64  `json:"encrypted_size"`
	SHA256        string `json:"sha256"`
}

type uploadManifestJSON struct {
	EncryptedSize int64  `json:"encrypted_size"`
	SHA256        string `json:"sha256"`
}

func uploadStatusResponse(secret *domain.Secret, objects []domain.SecretObject) map[string]any {
	chunks := make([]uploadObjectJSON, 0)
	var manifest *uploadManifestJSON

	for _, object := range objects {
		switch object.ObjectKind {
		case domain.ObjectKindManifest:
			manifest = &uploadManifestJSON{
				EncryptedSize: object.EncryptedSize,
				SHA256:        object.SHA256Hex,
			}
		case domain.ObjectKindChunk:
			chunks = append(chunks, uploadObjectJSON{
				Index:         object.ObjectIndex,
				EncryptedSize: object.EncryptedSize,
				SHA256:        object.SHA256Hex,
			})
		}
	}

	var uploadExpiresAt string
	if secret.UploadExpiresAt != nil {
		uploadExpiresAt = secret.UploadExpiresAt.UTC().Format(time.RFC3339)
	}

	return map[string]any{
		"public_id":            secret.PublicID,
		"upload_expires_at":    uploadExpiresAt,
		"chunk_size":           secret.ChunkSize,
		"chunk_count":          secret.ChunkCount,
		"encrypted_total_size": secret.EncryptedTotalSize,
		"chunks":               chunks,
		"manifest":             manifest,
	}
}

func uploadObjectResponse(object domain.SecretObject) map[string]any {
	return map[string]any{
		"index":          object.ObjectIndex,
		"encrypted_size": object.EncryptedSize,
		"sha256":         object.SHA256Hex,
	}
}

func splitUploadObjects(objects []domain.SecretObject) (*domain.SecretObject, map[int32]domain.SecretObject) {
	chunks := make(map[int32]domain.SecretObject)
	var manifest *domain.SecretObject
	for _, object := range objects {
		switch object.ObjectKind {
		case domain.ObjectKindManifest:
			copy := object
			manifest = &copy
		case domain.ObjectKindChunk:
			chunks[object.ObjectIndex] = object
		}
	}
	return manifest, chunks
}

func maxEncryptedChunkSize(secret *domain.Secret, maxFileSize int64) int64 {
	maxSize := secret.ChunkSize + chunkBodySlackBytes
	if maxSize <= 0 || maxSize > maxFileSize {
		return maxFileSize
	}
	return maxSize
}

func parseChunkIndex(value string) (int32, error) {
	if value == "" {
		return 0, errors.New("missing chunk index")
	}
	index, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, errors.New("malformed chunk index")
	}
	return int32(index), nil
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func secretStorageVersion(secret *domain.Secret) string {
	if secret.StorageVersion == "" {
		return domain.StorageVersionSingle
	}
	return secret.StorageVersion
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

func chunkedObjectKey(publicID, objectKind string, objectIndex int32) string {
	if objectKind == domain.ObjectKindManifest {
		return storageKey(publicID) + "/manifest"
	}
	return storageKey(publicID) + "/chunks/" + strconv.FormatInt(int64(objectIndex), 10)
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
