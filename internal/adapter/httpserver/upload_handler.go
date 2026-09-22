package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	storageKey := domain.UploadStorageKey(sessionID)
	uploadID, err := h.fileStore.CreateMultipartUpload(ctx, storageKey)
	if err != nil {
		return apperrors.InternalError("failed to create multipart upload", err)
	}

	now := time.Now()
	session := &domain.UploadSession{
		SessionID:         sessionID,
		UploadTokenHash:   tokencrypto.TokenHash(uploadToken),
		PublicID:          req.PublicID,
		StorageKey:        storageKey,
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

	return c.JSON(http.StatusCreated, uploadSessionResponse(session, uploadToken))
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
	if err := validatePartPlacement(session.BlobSize, partNumber, offset, size); err != nil {
		return apperrors.BadRequestError(err.Error())
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

	partFile, cleanup, err := spoolValidatedPart(c, size, partSHA256)
	if err != nil {
		return err
	}
	defer cleanup()

	etag, err := h.fileStore.UploadPart(
		c.Request().Context(),
		session.StorageKey,
		session.S3UploadID,
		partNumber,
		partFile,
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
	session, _, _, err := h.authenticateUploadSession(c)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	// stored records that storage assembled the object, so a later failure
	// leaves an object that no secret references.
	stored := false
	completed, err := h.repo.CompleteUploadSession(ctx, session.SessionID, time.Now(), func(locked *domain.UploadSession, parts []domain.UploadPart) error {
		if time.Now().After(locked.UploadExpiresAt) {
			return apperrors.ConflictError("upload session has expired")
		}
		completedParts, err := validateUploadParts(locked, parts)
		if err != nil {
			return apperrors.BadRequestError(err.Error())
		}
		if err := h.fileStore.CompleteMultipartUpload(ctx, locked.StorageKey, locked.S3UploadID, completedParts); err != nil {
			return err
		}
		stored = true
		return nil
	})
	if err != nil {
		return h.completeUploadFailed(ctx, session, stored, err)
	}

	// A repeated complete finds the session already completed and answers the
	// same way without creating anything.
	if stored {
		h.metrics.SecretsCreated.Inc()
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"expires_at": completed.SecretExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *UploadHandler) completeUploadFailed(ctx context.Context, session *domain.UploadSession, stored bool, err error) error {
	if stored {
		h.discardUpload(ctx, session)
		if errors.Is(err, domain.ErrDuplicate) {
			return apperrors.ConflictError("upload session cannot be completed")
		}
		return apperrors.InternalError("failed to create secret", err)
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperrors.NotFoundError("upload session not found")
	case errors.Is(err, domain.ErrConflict):
		return apperrors.ConflictError("upload session is not pending")
	case errors.Is(err, domain.ErrInvalidParts):
		// Recorded parts no longer match storage (for example a concurrent
		// re-upload of the same part number with different content). Forget
		// them so the client can upload the parts again.
		if clearErr := h.repo.ClearUploadParts(ctx, session.SessionID); clearErr != nil {
			return apperrors.InternalError("failed to reset upload parts", clearErr)
		}
		return apperrors.ConflictError("uploaded parts were rejected by storage; upload the parts again")
	case errors.Is(err, domain.ErrUploadNotFound):
		h.discardUpload(ctx, session)
		return apperrors.ConflictError("upload session is no longer valid; start a new upload")
	}
	if appErr, ok := errors.AsType[*apperrors.Error](err); ok {
		return appErr
	}
	return apperrors.InternalError("failed to complete multipart upload", err)
}

// discardUpload ends a session whose upload can no longer become a secret and
// removes whatever storage holds under its key. The object is only deleted
// once this call has moved the session to aborted: from then on no secret can
// ever reference the key. If the session was completed by a concurrent
// request in the meantime, the object belongs to that secret and stays.
func (h *UploadHandler) discardUpload(ctx context.Context, session *domain.UploadSession) {
	if err := h.repo.AbortUploadSession(ctx, session.SessionID, time.Now()); err != nil {
		// Still pending (for example the database is down): cleanup deletes
		// the object once the session expires.
		if !errors.Is(err, domain.ErrConflict) {
			slog.ErrorContext(ctx, "failed to abort upload session", "session_id", session.SessionID, "error", err)
		}
		return
	}
	if err := h.fileStore.Delete(ctx, session.StorageKey); err != nil {
		slog.ErrorContext(ctx, "failed to delete discarded upload object", "session_id", session.SessionID, "error", err)
	}
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
		if err := h.fileStore.AbortMultipartUpload(c.Request().Context(), session.StorageKey, session.S3UploadID); err != nil {
			return apperrors.InternalError("failed to abort multipart upload", err)
		}
		err := h.repo.AbortUploadSession(c.Request().Context(), session.SessionID, time.Now())
		if errors.Is(err, domain.ErrConflict) {
			// A concurrent complete or abort ended the session first.
			return apperrors.ConflictError("upload session is not pending")
		}
		if err != nil {
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

func uploadSessionResponse(session *domain.UploadSession, uploadToken string) map[string]any {
	return map[string]any{
		"session_id":        session.SessionID,
		"public_id":         session.PublicID,
		"part_size":         multipartUploadPartSize,
		"blob_size":         session.BlobSize,
		"expires_at":        session.SecretExpiresAt.UTC().Format(time.RFC3339),
		"upload_expires_at": session.UploadExpiresAt.UTC().Format(time.RFC3339),
		"state":             session.State,
		"upload_token":      uploadToken,
	}
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

// validatePartPlacement rejects parts that cannot belong to a valid final
// layout of blobSize bytes. Without it a session declaring a tiny blob could
// push thousands of full-size parts into storage until the session expires.
func validatePartPlacement(blobSize int64, partNumber int, offset, size int64) error {
	maxParts := (blobSize + s3MinimumPartSize - 1) / s3MinimumPartSize
	if maxParts < 1 {
		maxParts = 1
	}
	if int64(partNumber) > maxParts {
		return errors.New("part number exceeds the number of parts for the declared blob size")
	}
	// Every part before this one is non-final and therefore at least the S3
	// minimum, so the offset has a hard lower bound.
	if offset < int64(partNumber-1)*s3MinimumPartSize {
		return errors.New("invalid " + HeaderPartOffset + " header")
	}
	if offset+size > blobSize {
		return errors.New("part exceeds the declared blob size")
	}
	if offset+size < blobSize && size < s3MinimumPartSize {
		return errors.New("non-final part is below the minimum part size")
	}
	return nil
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

func spoolValidatedPart(c echo.Context, expectedSize int64, expectedSHA256 string) (*os.File, func(), error) {
	partFile, err := os.CreateTemp("", "secretli-upload-part-*")
	if err != nil {
		return nil, nil, apperrors.InternalError("failed to create temporary upload part", err)
	}
	cleanup := func() {
		_ = partFile.Close()
		_ = os.Remove(partFile.Name())
	}

	body := http.MaxBytesReader(c.Response(), c.Request().Body, expectedSize+1)
	hasher := sha256.New()
	written, err := io.Copy(partFile, io.TeeReader(body, hasher))
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		cleanup()
		return nil, nil, apperrors.BadRequestError("part exceeds declared size")
	}
	if err != nil {
		cleanup()
		return nil, nil, apperrors.BadRequestError("failed to read part body")
	}
	if written > expectedSize {
		cleanup()
		return nil, nil, apperrors.BadRequestError("part exceeds declared size")
	}
	if written != expectedSize {
		cleanup()
		return nil, nil, apperrors.BadRequestError("request body size does not match " + HeaderPartSize)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != expectedSHA256 {
		cleanup()
		return nil, nil, apperrors.BadRequestError("part SHA-256 mismatch")
	}
	if _, err := partFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, apperrors.InternalError("failed to prepare upload part", err)
	}
	return partFile, cleanup, nil
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
