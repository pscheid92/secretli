package errors

import (
	"fmt"
	"net/http"
	"testing"
)

func TestErrorType_HTTPStatus(t *testing.T) {
	tests := []struct {
		errType ErrorType
		status  int
	}{
		{BadRequest, http.StatusBadRequest},
		{NotFound, http.StatusNotFound},
		{Conflict, http.StatusConflict},
		{Forbidden, http.StatusForbidden},
		{Internal, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		e := &Error{Type: tt.errType, Message: "test"}
		if got := e.HTTPStatus(); got != tt.status {
			t.Errorf("HTTPStatus(%q) = %d, want %d", tt.errType, got, tt.status)
		}
	}
}

func TestError_Error(t *testing.T) {
	t.Run("without cause", func(t *testing.T) {
		e := BadRequestError("invalid input")
		if got := e.Error(); got != "invalid input" {
			t.Errorf("Error() = %q, want %q", got, "invalid input")
		}
	})

	t.Run("with cause", func(t *testing.T) {
		cause := fmt.Errorf("connection refused")
		e := InternalError("database error", cause)
		want := "database error: connection refused"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}

func TestError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	e := InternalError("wrapper", cause)
	if got := e.Unwrap(); got != cause {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}

	e2 := BadRequestError("no cause")
	if got := e2.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestError_ToResponse(t *testing.T) {
	e := &Error{
		Type:    BadRequest,
		Message: "validation failed",
		Context: map[string]any{"details": []string{"field is required"}},
	}
	resp := e.ToResponse()
	if resp.Error != "validation failed" {
		t.Errorf("Error = %q, want %q", resp.Error, "validation failed")
	}
	if resp.Details == nil {
		t.Error("expected non-nil Details")
	}
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		errType ErrorType
		msg     string
	}{
		{"BadRequest", BadRequestError("bad"), BadRequest, "bad"},
		{"NotFound", NotFoundError("missing"), NotFound, "missing"},
		{"Forbidden", ForbiddenError("denied"), Forbidden, "denied"},
		{"Conflict", ConflictError("dup"), Conflict, "dup"},
		{"Internal", InternalError("fail", fmt.Errorf("cause")), Internal, "fail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Type != tt.errType {
				t.Errorf("Type = %q, want %q", tt.err.Type, tt.errType)
			}
			if tt.err.Message != tt.msg {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.msg)
			}
		})
	}
}

func TestAsAppError(t *testing.T) {
	t.Run("passes through *Error", func(t *testing.T) {
		original := NotFoundError("not here")
		got := AsAppError(original)
		if got != original {
			t.Error("expected same *Error back")
		}
	})

	t.Run("wraps unknown error as Internal", func(t *testing.T) {
		got := AsAppError(fmt.Errorf("something broke"))
		if got.Type != Internal {
			t.Errorf("Type = %q, want %q", got.Type, Internal)
		}
		if got.Cause == nil {
			t.Error("expected non-nil Cause")
		}
	})
}
