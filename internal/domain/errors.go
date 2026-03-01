package domain

import "errors"

// Sentinel errors returned by repository implementations.
var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
)
