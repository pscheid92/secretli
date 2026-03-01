## Why

Request validation is scattered across handlers as manual if-checks with generic error messages like "missing required fields". Adopting `go-playground/validator` centralizes validation rules on struct tags, reduces handler boilerplate, and enables field-specific error messages so clients know exactly which field failed and why.

## What Changes

- Add `github.com/go-playground/validator/v10` as a direct dependency
- Add `validate` struct tags to `CreateSecretRequest` and `fileMetadata` structs
- Create a shared validation helper that runs the validator and translates errors into field-specific JSON error responses
- Replace manual validation checks in `CreateSecret` and `UploadFile` handlers with validator calls
- Error responses for validation failures will include the failing field name (e.g., `"public_id is required"` instead of `"missing required fields"`)
- **BREAKING**: Validation error response format changes from `{"error": "..."}` to `{"error": "validation failed", "details": [...]}`

## Capabilities

### New Capabilities

_None — this replaces existing validation logic, not a new feature._

### Modified Capabilities

- `secret-crud`: Validation error responses become field-specific; `CreateSecretRequest` uses struct tag validation instead of manual checks
- `file-handlers`: File upload metadata validation uses struct tag validation instead of manual checks

## Impact

- **Dependencies**: New direct dependency `github.com/go-playground/validator/v10`
- **Handlers**: `internal/handler/secret.go` and `internal/handler/file.go` — validation logic replaced
- **Model**: `internal/model/secret.go` — `validate` tags added to `CreateSecretRequest`
- **Handler helpers**: New validation helper function in handler package
- **Tests**: `internal/handler/secret_test.go` and `internal/handler/file_test.go` — error response assertions updated for new format
- **API**: Clients receiving 400 errors will see a different JSON shape with field-level detail
