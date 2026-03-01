## Context

Request validation in the handlers is currently manual: sequential if-checks for empty strings, size limits, and expiration enums. Error messages are generic ("missing required fields") without identifying which field failed. Both `CreateSecret` and `UploadFile` handlers duplicate the same validation patterns (required field checks, expiration parsing). The `fileMetadata` struct in `file.go` is a handler-local type that mirrors `CreateSecretRequest` fields.

## Goals / Non-Goals

**Goals:**
- Replace manual validation in `CreateSecret` and `UploadFile` with `go-playground/validator` struct tags
- Return field-specific validation errors so clients know exactly what failed
- Centralize validation rules on the request structs themselves
- Add a custom validator for the expiration enum to eliminate the manual `parseExpiration` check during validation

**Non-Goals:**
- Validating URL params or headers (publicID, X-Retrieval-Token, X-Deletion-Token) — these remain simple single-field checks in handlers
- Changing the `parseExpiration` function itself — it's still needed to convert the string to a duration after validation passes
- Adding validation middleware — validation stays in the handler since each endpoint has different input shapes (JSON body vs multipart form)
- Validating the `Secret` model struct (domain model, not a request DTO)

## Decisions

### Single shared `validator.Validate` instance

Create one `*validator.Validate` instance at startup (in a `validate.go` helper file in the handler package) and reuse it. The validator is safe for concurrent use. Register the custom `expiration` validator at init time.

**Alternative**: Create a new validator per request — rejected because `validator.New()` is expensive (reflection setup).

### Custom `expiration` validator tag

Register a custom validation function that checks membership in the `expirationDurations` map. This way `Expiration string \`validate:"required,expiration"\`` handles both "required" and "valid enum" in one pass, removing the separate `parseExpiration` call during validation.

**Alternative**: Use `oneof=5m 10m 15m 1h 4h 12h 1d 3d 7d` built-in tag — rejected because it duplicates the expiration list (already in the map) and won't stay in sync if values change.

### Structured validation error response

Change the 400 response for validation failures from:
```json
{"error": "missing required fields"}
```
To:
```json
{"error": "validation failed", "details": ["public_id is required", "expiration must be a valid duration"]}
```

This gives clients actionable field-level feedback. The `details` array keeps things simple — no nested objects or error codes. The existing `writeError` function stays for non-validation errors (404, 403, 500).

**Alternative**: Return per-field object `{"fields": {"public_id": "required"}}` — rejected as over-engineered for this API's needs.

### Move `fileMetadata` to model package and add validate tags

The `fileMetadata` struct currently lives in `file.go`. Move it to `model/secret.go` as `CreateFileRequest` alongside `CreateSecretRequest` to centralize request types. Both structs get `validate` tags.

**Alternative**: Keep `fileMetadata` in handler and add tags there — rejected because it breaks the pattern of request types living in the model package.

## Risks / Trade-offs

- **[Breaking API change]** → Clients parsing `{"error": "..."}` for validation will see a different shape. Acceptable since this is an early-stage project with no external consumers.
- **[New dependency]** → `go-playground/validator` is widely adopted (13k+ stars, used by Gin). Low risk.
- **[Tag-based max length for encrypted_data]** → The `max=1048576` tag checks string length in bytes which matches the current `len()` check. Works correctly since `EncryptedData` is base64-encoded (ASCII).
