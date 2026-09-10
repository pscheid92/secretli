package domain

import "errors"

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")

	// ErrInvalidParts is returned by a MultipartFileStore when the storage
	// backend rejects the recorded parts on completion (stale ETag, wrong
	// order, undersized part). The parts must be uploaded again.
	ErrInvalidParts = errors.New("invalid multipart parts")
	// ErrUploadNotFound is returned by a MultipartFileStore when the backend
	// no longer knows the multipart upload (already completed or aborted).
	ErrUploadNotFound = errors.New("multipart upload not found")
)
