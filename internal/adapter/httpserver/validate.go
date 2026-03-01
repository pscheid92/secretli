package httpserver

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	apperrors "github.com/pscheid92/secretli/internal/platform/errors"
)

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		tag := fld.Tag.Get("form")
		split := strings.SplitN(tag, ",", 2)
		name := split[0]

		if name == "-" {
			return ""
		}

		return name
	})

	_ = v.RegisterValidation("expiration", func(fl validator.FieldLevel) bool {
		field := fl.Field().String()
		_, ok := expirationDurations[field]
		return ok
	})

	return v
}

func (h *SecretHandler) validateRequest(v any) []string {
	err := h.validate.Struct(v)
	if err == nil {
		// POSITIVE: no errors detected
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

func fieldErrorMessage(fe validator.FieldError) string {
	field := fe.Field()
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "max":
		return fmt.Sprintf("%s exceeds maximum length of %s", field, fe.Param())
	case "expiration":
		return fmt.Sprintf("%s must be a valid duration (5m, 10m, 15m, 1h, 4h, 12h, 1d, 3d, 7d)", field)
	default:
		return fmt.Sprintf("%s is invalid", field)
	}
}

func validationError(details []string) *apperrors.Error {
	return &apperrors.Error{
		Type:    apperrors.BadRequest,
		Message: "validation failed",
		Context: map[string]any{"details": details},
	}
}
