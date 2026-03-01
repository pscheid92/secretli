package handler

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	validate.RegisterValidation("expiration", func(fl validator.FieldLevel) bool {
		_, ok := expirationDurations[fl.Field().String()]
		return ok
	})
}

func validateRequest(v any) []string {
	err := validate.Struct(v)
	if err == nil {
		return nil
	}

	var details []string
	for _, fe := range err.(validator.ValidationErrors) {
		details = append(details, fieldErrorMessage(fe))
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

func writeValidationError(w http.ResponseWriter, details []string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":   "validation failed",
		"details": details,
	})
}
