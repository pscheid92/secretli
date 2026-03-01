## 1. Add dependency

- [x] 1.1 Run `go get github.com/go-playground/validator/v10` to add the dependency

## 2. Request structs and validate tags

- [x] 2.1 Add `validate` struct tags to `CreateSecretRequest` in `internal/model/secret.go` (`required`, `max=1048576` for encrypted_data)
- [x] 2.2 Move `fileMetadata` from `internal/handler/file.go` to `internal/model/secret.go` as `CreateFileRequest` with `validate` struct tags

## 3. Validation helper

- [x] 3.1 Create `internal/handler/validate.go` with a shared `*validator.Validate` instance, custom `expiration` tag registration, and a `validateRequest` helper that returns field-specific error messages
- [x] 3.2 Add `writeValidationError` function that writes `{"error": "validation failed", "details": [...]}` response

## 4. Update handlers

- [x] 4.1 Update `CreateSecret` in `internal/handler/secret.go` — replace manual required-field checks, size check, and expiration validation with `validateRequest(&req)` call
- [x] 4.2 Update `UploadFile` in `internal/handler/file.go` — replace manual required-field checks and expiration validation with `validateRequest(&meta)` call, using the new `CreateFileRequest` type

## 5. Update tests

- [x] 5.1 Update `internal/handler/secret_test.go` — no changes needed (tests assert on status codes only)
- [x] 5.2 Update `internal/handler/file_test.go` — no changes needed (tests assert on status codes only)

## 6. Verification

- [x] 6.1 Run `go build ./...` to confirm compilation
- [x] 6.2 Run `go test ./internal/handler/...` to confirm all 42 tests pass
