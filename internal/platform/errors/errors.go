package errors

import (
	"fmt"
	"net/http"
)

// ErrorType classifies application errors for HTTP status mapping.
type ErrorType string

const (
	BadRequest ErrorType = "bad_request"
	NotFound   ErrorType = "not_found"
	Conflict   ErrorType = "conflict"
	Forbidden  ErrorType = "forbidden"
	Internal   ErrorType = "internal"
)

// Error is a structured application error that maps to an HTTP response.
type Error struct {
	Type    ErrorType
	Message string
	Cause   error
	Context map[string]any
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// HTTPStatus returns the HTTP status code for this error type.
func (e *Error) HTTPStatus() int {
	switch e.Type {
	case BadRequest:
		return http.StatusBadRequest
	case NotFound:
		return http.StatusNotFound
	case Conflict:
		return http.StatusConflict
	case Forbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// ErrorResponse is the JSON structure returned to clients.
type ErrorResponse struct {
	Error   string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

// ToResponse converts the error to an HTTP-safe JSON response body.
func (e *Error) ToResponse() ErrorResponse {
	return ErrorResponse{
		Error:   e.Message,
		Details: e.Context,
	}
}

// --- Constructors ---

func BadRequestError(msg string) *Error {
	return &Error{Type: BadRequest, Message: msg}
}

func NotFoundError(msg string) *Error {
	return &Error{Type: NotFound, Message: msg}
}

func ForbiddenError(msg string) *Error {
	return &Error{Type: Forbidden, Message: msg}
}

func ConflictError(msg string) *Error {
	return &Error{Type: Conflict, Message: msg}
}

func InternalError(msg string, cause error) *Error {
	return &Error{Type: Internal, Message: msg, Cause: cause}
}

// AsAppError converts any error to an *Error, wrapping unknown errors as Internal.
func AsAppError(err error) *Error {
	if appErr, ok := err.(*Error); ok {
		return appErr
	}
	return InternalError("internal server error", err)
}
