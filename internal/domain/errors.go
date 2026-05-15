package domain

import "errors"

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
)
