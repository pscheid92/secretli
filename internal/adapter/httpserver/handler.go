package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"

	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

// HandlerFunc is an HTTP handler that returns an error instead of handling it inline.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler, calling handleError for non-nil errors.
func (fn HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		handleError(w, r, err)
	}
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := apperrors.AsAppError(err)

	if appErr.Type == apperrors.Internal {
		slog.ErrorContext(r.Context(), appErr.Message, "error", appErr.Cause)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.HTTPStatus())
	_ = json.NewEncoder(w).Encode(appErr.ToResponse())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func validationError(details []string) *apperrors.Error {
	return &apperrors.Error{
		Type:    apperrors.BadRequest,
		Message: "validation failed",
		Context: map[string]any{"details": details},
	}
}
