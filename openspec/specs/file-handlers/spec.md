## ADDED Requirements

### Requirement: Upload encrypted file
The server SHALL accept `POST /api/v1/secrets/file` as `multipart/form-data` with two parts: a `metadata` JSON part containing `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `expiration`, `burn_after_read`, `password_protected`, and `encrypted_filename`; and a `file` binary part containing the encrypted blob. The server SHALL stream the file part directly to S3 at key `secrets/{public_id}` without buffering the full file in memory. The server SHALL enforce a maximum file size of 100MB via `http.MaxBytesReader`. On success, the server SHALL return `201 Created` with `{ "expires_at": "<ISO8601>" }`.

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

#### Scenario: Invalid metadata fields
- **WHEN** required fields are missing from the metadata JSON (public_id, retrieval_token, deletion_token, nonce, expiration)
- **THEN** the response status is 400 Bad Request

#### Scenario: Duplicate public_id rejected
- **WHEN** the metadata contains a `public_id` that already exists in the database
- **THEN** the response status is 409 Conflict

### Requirement: Download encrypted file
The server SHALL accept `POST /api/v1/secrets/{publicID}/file` with an `X-Retrieval-Token` header. The server SHALL verify the token, stream the encrypted file from S3 to the response body as `application/octet-stream`, and include the `X-Encrypted-Filename` header. If `burn_after_read` is true, the secret and S3 object SHALL be deleted after successful streaming.

#### Scenario: Successful file download
- **WHEN** a POST request with a valid retrieval token is made for a file secret
- **THEN** the encrypted file is streamed from S3 to the response body
- **AND** the response Content-Type is `application/octet-stream`
- **AND** the `X-Encrypted-Filename` header contains the encrypted filename

#### Scenario: Invalid retrieval token
- **WHEN** the `X-Retrieval-Token` does not match the stored hash
- **THEN** the response status is 403 Forbidden

#### Scenario: File secret not found
- **WHEN** the `publicID` does not exist or has expired
- **THEN** the response status is 404 Not Found

#### Scenario: Burn-after-read file
- **WHEN** a burn-after-read file secret is downloaded successfully
- **THEN** the encrypted file is streamed to the client
- **AND** the database record and S3 object are deleted afterward

#### Scenario: Text secret accessed via file endpoint
- **WHEN** the file download endpoint is called for a text-type secret
- **THEN** the response status is 400 Bad Request with a message indicating wrong secret type
