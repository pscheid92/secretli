## MODIFIED Requirements

### Requirement: Upload encrypted file
The server SHALL accept `POST /api/v1/secrets/file` as `multipart/form-data` with two parts: a `metadata` JSON part and a `file` binary part. The `metadata` JSON SHALL be validated using struct tag validation on `CreateFileRequest` with required fields: `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `expiration`, and `encrypted_filename`. The `expiration` field MUST be a valid duration string. On validation failure, the server SHALL return `400 Bad Request` with `{"error": "validation failed", "details": ["<field-specific messages>"]}`. The server SHALL stream the file part directly to S3 at key `secrets/{public_id}` without buffering the full file in memory. The server SHALL enforce a maximum file size of 100MB via `http.MaxBytesReader`. On success, the server SHALL return `201 Created` with `{ "expires_at": "<ISO8601>" }`.

#### Scenario: Successful file upload
- **WHEN** a multipart POST is sent with valid metadata and an encrypted file blob
- **THEN** the file is streamed to S3, a database record is created with `secret_type: "file"`, and the response is 201 with `expires_at`

#### Scenario: File exceeds size limit
- **WHEN** the file part exceeds 100MB
- **THEN** the response status is 400 Bad Request with an error message about file size

#### Scenario: Missing metadata part
- **WHEN** the multipart request is missing the `metadata` part
- **THEN** the response status is 400 Bad Request

#### Scenario: Missing file part
- **WHEN** the multipart request is missing the `file` part
- **THEN** the response status is 400 Bad Request

#### Scenario: Missing required metadata fields rejected with field-specific errors
- **WHEN** required fields are missing from the metadata JSON (e.g., `public_id` and `nonce`)
- **THEN** the response status is 400 Bad Request
- **AND** the response body contains `{"error": "validation failed", "details": [...]}`
- **AND** the `details` array contains messages identifying the missing fields

#### Scenario: Invalid metadata expiration rejected
- **WHEN** the `expiration` field in metadata is not a valid duration
- **THEN** the response status is 400 Bad Request
- **AND** the response body contains `"details"` with a message identifying the `expiration` field

#### Scenario: Duplicate public_id rejected
- **WHEN** the metadata contains a `public_id` that already exists in the database
- **THEN** the response status is 409 Conflict
