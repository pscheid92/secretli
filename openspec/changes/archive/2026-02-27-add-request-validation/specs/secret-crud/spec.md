## MODIFIED Requirements

### Requirement: Create a text secret
The server SHALL accept `POST /api/v1/secrets` with a JSON body containing `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `encrypted_data`, `expiration`, `burn_after_read`, and `password_protected`. The server SHALL validate the request using struct tag validation on `CreateSecretRequest`. All required fields (`public_id`, `retrieval_token`, `deletion_token`, `nonce`, `encrypted_data`, `expiration`) MUST be non-empty. The `encrypted_data` field MUST NOT exceed 1MB. The `expiration` field MUST be a valid duration string. On validation failure, the server SHALL return `400 Bad Request` with `{"error": "validation failed", "details": ["<field-specific messages>"]}`. The server SHALL hash the tokens with SHA-256 before storing. The server SHALL convert the `expiration` string to an absolute timestamp. On success, the server SHALL return `201 Created` with `{"expires_at": "<ISO8601>"}`.

#### Scenario: Successful secret creation
- **WHEN** a POST request is made to `/api/v1/secrets` with valid JSON body
- **THEN** the secret is stored in the database with hashed tokens
- **AND** the response status is 201 with an `expires_at` field

#### Scenario: Duplicate public_id rejected
- **WHEN** a POST request uses a `public_id` that already exists
- **THEN** the response status is 409 Conflict

#### Scenario: Invalid expiration rejected
- **WHEN** the `expiration` field is not one of `5m`, `10m`, `15m`, `1h`, `4h`, `12h`, `1d`, `3d`, `7d`
- **THEN** the response status is 400 Bad Request
- **AND** the response body contains `"details"` with a message identifying the `expiration` field

#### Scenario: Missing required fields rejected with field-specific errors
- **WHEN** the JSON body is missing `public_id` and `nonce`
- **THEN** the response status is 400 Bad Request
- **AND** the response body contains `{"error": "validation failed", "details": [...]}`
- **AND** the `details` array contains messages identifying both `public_id` and `nonce`

#### Scenario: Secret size limit enforced
- **WHEN** the `encrypted_data` field exceeds 1MB (base64-encoded)
- **THEN** the response status is 400 Bad Request
- **AND** the response body contains `"details"` with a message identifying the `encrypted_data` field
