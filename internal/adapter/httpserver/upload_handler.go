package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"

	"github.com/pscheid92/secretli/internal/adapter/metrics"
	"github.com/pscheid92/secretli/internal/domain"
	tokencrypto "github.com/pscheid92/secretli/internal/platform/crypto"
	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

const (
	HeaderPartOffset = "X-Part-Offset"
	HeaderPartSize   = "X-Part-Size"
	HeaderPartSHA256 = "X-Part-SHA256"

	uploadSessionTTL        = 24 * time.Hour
	multipartUploadPartSize = 32 * 1024 * 1024
	maxMultipartUploadPart  = multipartUploadPartSize + 1024*1024
	s3MinimumPartSize       = 5 * 1024 * 1024
	maxMultipartPartNumber  = 10000
)

type UploadHandler struct {
	repo        domain.UploadSessionRepo
	fileStore   domain.MultipartFileStore
	maxFileSize int64
	metrics     *metrics.SecretMetrics
	validate    *validator.Validate
}

type createUploadSessionRequest struct {
	PublicID      string `json:"public_id" validate:"required,public_id"`
	MetadataToken string `json:"metadata_token" validate:"required,secret_token"`
	BlobToken     string `json:"blob_token" validate:"required,secret_token"`
	DeletionToken string `json:"deletion_token" validate:"required,secret_token"`
	EncryptedMeta string `json:"encrypted_meta" validate:"required,encrypted_meta"`
	Expiration    string `json:"expiration" validate:"required,expiration"`
	BurnAfterRead bool   `json:"burn_after_read"`
	BlobSize      int64  `json:"blob_size"`
}

func NewUploadHandler(repo domain.UploadSessionRepo, fileStore domain.MultipartFileStore, maxFileSize int64, m *metrics.SecretMetrics) *UploadHandler {
	return &UploadHandler{
		repo:        repo,
		fileStore:   fileStore,
		maxFileSize: maxFileSize,
		metrics:     m,
		validate:    newValidator(),
	}
}

func (h *UploadHandler) CreateUploadSession(c echo.Context) error {
	var req createUploadSessionRequest
	if err := c.Bind(&req); err != nil {
		return apperrors.BadRequestError("invalid request body")
	}
	if details := h.validateRequest(&req); details != nil {
		return validationError(details)
	}
	if req.BlobSize <= 0 {
		return apperrors.BadRequestError("blob_size must be positive")
	}
	if req.BlobSize > h.maxFileSize {
		return apperrors.BadRequestError("file exceeds maximum size limit")
	}

	duration, err := parseExpiration(req.Expiration)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}

	sessionID, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create upload session", err)
	}
	uploadToken, err := newRetrievalSessionToken()
	if err != nil {
		return apperrors.InternalError("failed to create upload session", err)
	}

	ctx := c.Request().Context()
	storageKey := domain.SecretStorageKey(req.PublicID)
	uploadID, err := h.fileStore.CreateMultipartUpload(ctx, storageKey)
	if err != nil {
		return apperrors.InternalError("failed to create multipart upload", err)
	}

	now := time.Now()
	session := &domain.UploadSession{
		SessionID:         sessionID,
		UploadTokenHash:   tokencrypto.TokenHash(uploadToken),
		PublicID:          req.PublicID,
		S3UploadID:        uploadID,
		BlobSize:          req.BlobSize,
		MetadataTokenHash: tokencrypto.TokenHash(req.MetadataToken),
		BlobTokenHash:     tokencrypto.TokenHash(req.BlobToken),
		DeletionTokenHash: tokencrypto.TokenHash(req.DeletionToken),
		EncryptedMeta:     req.EncryptedMeta,
		BurnAfterRead:     req.BurnAfterRead,
		SecretExpiresAt:   now.Add(duration),
		UploadExpiresAt:   now.Add(uploadSessionTTL),
		State:             domain.UploadSessionStatePending,
		CreatedAt:         now,
	}

	if err := h.repo.CreateUploadSession(ctx, session); err != nil {
		_ = h.fileStore.AbortMultipartUpload(ctx, storageKey, uploadID)
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("secret with this public_id already exists")
		}
		return apperrors.InternalError("failed to create upload session", err)
	}

	return c.JSON(http.StatusCreated, uploadSessionResponse(session, nil, uploadToken))
}

func (h *UploadHandler) UploadSessionStatus(c echo.Context) error {
	session, parts, _, err := h.authenticateUploadSession(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, uploadSessionResponse(session, parts, ""))
}

func (h *UploadHandler) UploadPart(c echo.Context) error {
	session, parts, _, err := h.authenticateUploadSession(c)
	if err != nil {
		return err
	}
	if err := validatePendingUploadSession(session); err != nil {
		return err
	}

	partNumber, err := parsePartNumber(c.Param("partNumber"))
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}
	offset, err := parseInt64Header(c.Request(), HeaderPartOffset)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}
	size, err := parseInt64Header(c.Request(), HeaderPartSize)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}
	if size <= 0 || size > maxMultipartUploadPart || size > h.maxFileSize {
		return apperrors.BadRequestError("invalid " + HeaderPartSize + " header")
	}
	if c.Request().ContentLength >= 0 && c.Request().ContentLength != size {
		return apperrors.BadRequestError("request body size does not match " + HeaderPartSize)
	}
	partSHA256 := c.Request().Header.Get(HeaderPartSHA256)
	if !isSHA256Hex(partSHA256) {
		return apperrors.BadRequestError("invalid " + HeaderPartSHA256 + " header")
	}

	for _, existing := range parts {
		if existing.PartNumber != partNumber {
			continue
		}
		if existing.Offset == offset && existing.Size == size && existing.SHA256 == partSHA256 {
			return c.JSON(http.StatusOK, uploadPartResponse(existing))
		}
		return apperrors.ConflictError("part already uploaded with different content")
	}

	body, err := readBoundedBody(c, size)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != partSHA256 {
		return apperrors.BadRequestError("part SHA-256 mismatch")
	}

	etag, err := h.fileStore.UploadPart(
		c.Request().Context(),
		domain.SecretStorageKey(session.PublicID),
		session.S3UploadID,
		partNumber,
		bytes.NewReader(body),
		size,
	)
	if err != nil {
		return apperrors.InternalError("failed to upload part to S3", err)
	}

	recorded, err := h.repo.RecordUploadPart(c.Request().Context(), &domain.UploadPart{
		SessionID:  session.SessionID,
		PartNumber: partNumber,
		Offset:     offset,
		Size:       size,
		SHA256:     partSHA256,
		ETag:       etag,
		CreatedAt:  time.Now(),
	})
	if errors.Is(err, domain.ErrConflict) {
		return apperrors.ConflictError("part already uploaded with different content")
	}
	if err != nil {
		return apperrors.InternalError("failed to record uploaded part", err)
	}

	return c.JSON(http.StatusOK, uploadPartResponse(*recorded))
}

func (h *UploadHandler) CompleteUploadSession(c echo.Context) error {
	session, parts, _, err := h.authenticateUploadSession(c)
	if err != nil {
		return err
	}
	if err := validatePendingUploadSession(session); err != nil {
		return err
	}

	completedParts, err := validateUploadParts(session, parts)
	if err != nil {
		return apperrors.BadRequestError(err.Error())
	}

	ctx := c.Request().Context()
	sk := domain.SecretStorageKey(session.PublicID)
	if err := h.fileStore.CompleteMultipartUpload(ctx, sk, session.S3UploadID, completedParts); err != nil {
		return apperrors.InternalError("failed to complete multipart upload", err)
	}

	secret := &domain.Secret{
		PublicID:          session.PublicID,
		MetadataTokenHash: session.MetadataTokenHash,
		BlobTokenHash:     session.BlobTokenHash,
		DeletionTokenHash: session.DeletionTokenHash,
		EncryptedMeta:     session.EncryptedMeta,
		BlobSize:          session.BlobSize,
		BurnAfterRead:     session.BurnAfterRead,
		ExpiresAt:         session.SecretExpiresAt,
	}
	if err := h.repo.CompleteUploadSession(ctx, session.SessionID, secret, time.Now()); err != nil {
		_ = h.fileStore.Delete(ctx, sk)
		if errors.Is(err, domain.ErrDuplicate) || errors.Is(err, domain.ErrConflict) {
			return apperrors.ConflictError("upload session cannot be completed")
		}
		return apperrors.InternalError("failed to create secret", err)
	}

	h.metrics.SecretsCreated.Inc()

	return c.JSON(http.StatusCreated, map[string]string{
		"expires_at": session.SecretExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *UploadHandler) AbortUploadSession(c echo.Context) error {
	session, _, _, err := h.authenticateUploadSession(c)
	if err != nil {
		return err
	}
	if session.State == domain.UploadSessionStateCompleted {
		return apperrors.ConflictError("upload session already completed")
	}
	if session.State == domain.UploadSessionStatePending {
		if err := h.fileStore.AbortMultipartUpload(c.Request().Context(), domain.SecretStorageKey(session.PublicID), session.S3UploadID); err != nil {
			return apperrors.InternalError("failed to abort multipart upload", err)
		}
		if err := h.repo.AbortUploadSession(c.Request().Context(), session.SessionID, time.Now()); err != nil {
			return apperrors.InternalError("failed to abort upload session", err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *UploadHandler) authenticateUploadSession(c echo.Context) (*domain.UploadSession, []domain.UploadPart, string, error) {
	sessionID := c.Param("sessionID")
	if sessionID == "" {
		return nil, nil, "", apperrors.BadRequestError("missing session_id")
	}
	if !domain.ValidToken(sessionID) {
		return nil, nil, "", apperrors.BadRequestError("malformed session_id")
	}

	uploadToken, err := bearerToken(c.Request().Header.Get("Authorization"))
	if err != nil {
		return nil, nil, "", apperrors.BadRequestError(err.Error())
	}
	if !domain.ValidToken(uploadToken) {
		return nil, nil, "", apperrors.BadRequestError("malformed Authorization header")
	}

	session, parts, err := h.repo.GetUploadSession(c.Request().Context(), sessionID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil, "", apperrors.NotFoundError("upload session not found")
	}
	if err != nil {
		return nil, nil, "", apperrors.InternalError("failed to get upload session", err)
	}
	if !tokencrypto.TokensEqual(tokencrypto.TokenHash(uploadToken), session.UploadTokenHash) {
		return nil, nil, "", apperrors.ForbiddenError("invalid upload token")
	}
	return session, parts, uploadToken, nil
}

func (h *UploadHandler) validateRequest(v any) []string {
	err := h.validate.Struct(v)
	if err == nil {
		return nil
	}
	errs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return []string{"unknown validation error"}
	}
	details := make([]string, len(errs))
	for i, fe := range errs {
		details[i] = fieldErrorMessage(fe)
	}
	return details
}

func uploadSessionResponse(session *domain.UploadSession, parts []domain.UploadPart, uploadToken string) map[string]any {
	uploadedParts := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		uploadedParts = append(uploadedParts, uploadPartResponse(part))
	}

	response := map[string]any{
		"session_id":        session.SessionID,
		"public_id":         session.PublicID,
		"part_size":         multipartUploadPartSize,
		"blob_size":         session.BlobSize,
		"expires_at":        session.SecretExpiresAt.UTC().Format(time.RFC3339),
		"upload_expires_at": session.UploadExpiresAt.UTC().Format(time.RFC3339),
		"state":             session.State,
		"uploaded_parts":    uploadedParts,
	}
	if uploadToken != "" {
		response["upload_token"] = uploadToken
	}
	return response
}

func uploadPartResponse(part domain.UploadPart) map[string]any {
	return map[string]any{
		"part_number": part.PartNumber,
		"offset":      part.Offset,
		"size":        part.Size,
		"sha256":      part.SHA256,
		"etag":        part.ETag,
	}
}

func validatePendingUploadSession(session *domain.UploadSession) error {
	if session.State != domain.UploadSessionStatePending {
		return apperrors.ConflictError("upload session is not pending")
	}
	if time.Now().After(session.UploadExpiresAt) {
		return apperrors.ConflictError("upload session has expired")
	}
	return nil
}

func validateUploadParts(session *domain.UploadSession, parts []domain.UploadPart) ([]domain.CompletedPart, error) {
	if len(parts) == 0 {
		return nil, errors.New("upload session has no parts")
	}
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	completed := make([]domain.CompletedPart, 0, len(parts))
	var expectedOffset int64
	for i, part := range parts {
		expectedPartNumber := i + 1
		if part.PartNumber != expectedPartNumber {
			return nil, fmt.Errorf("missing upload part %d", expectedPartNumber)
		}
		if part.Offset != expectedOffset {
			return nil, fmt.Errorf("upload part %d has invalid offset", part.PartNumber)
		}
		if i < len(parts)-1 && part.Size < s3MinimumPartSize {
			return nil, fmt.Errorf("upload part %d is below minimum size", part.PartNumber)
		}
		expectedOffset += part.Size
		if expectedOffset > session.BlobSize {
			return nil, errors.New("uploaded parts exceed expected size")
		}
		completed = append(completed, domain.CompletedPart{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		})
	}
	if expectedOffset != session.BlobSize {
		return nil, errors.New("upload is missing parts")
	}
	return completed, nil
}

func parsePartNumber(value string) (int, error) {
	partNumber, err := strconv.Atoi(value)
	if err != nil || partNumber <= 0 || partNumber > maxMultipartPartNumber {
		return 0, errors.New("invalid part number")
	}
	return partNumber, nil
}

func parseInt64Header(r *http.Request, header string) (int64, error) {
	value := r.Header.Get(header)
	if value == "" {
		return 0, errors.New("missing " + header + " header")
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid " + header + " header")
	}
	return parsed, nil
}

func readBoundedBody(c echo.Context, expectedSize int64) ([]byte, error) {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, expectedSize+1)
	body, err := io.ReadAll(c.Request().Body)
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return nil, apperrors.BadRequestError("part exceeds declared size")
	}
	if err != nil {
		return nil, apperrors.BadRequestError("failed to read part body")
	}
	if int64(len(body)) != expectedSize {
		return nil, apperrors.BadRequestError("request body size does not match " + HeaderPartSize)
	}
	return body, nil
}

func isSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' {
			continue
		}
		if r >= 'a' && r <= 'f' {
			continue
		}
		return false
	}
	return true
}
