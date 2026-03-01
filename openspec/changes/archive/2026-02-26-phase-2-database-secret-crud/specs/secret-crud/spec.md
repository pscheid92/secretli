## ADDED Requirements

### Requirement: Create a text secret
The server SHALL accept `POST /api/v1/secrets` with a JSON body containing `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `encrypted_data`, `expiration`, `burn_after_read`, and `password_protected`. The server SHALL hash the tokens with SHA-256 before storing. The server SHALL convert the `expiration` string to an absolute timestamp. On success, the server SHALL return `201 Created` with `{"expires_at": "<ISO8601>"}`.

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

#### Scenario: Missing required fields rejected
- **WHEN** the JSON body is missing `public_id`, `retrieval_token`, `deletion_token`, `nonce`, or `encrypted_data`
- **THEN** the response status is 400 Bad Request

#### Scenario: Secret size limit enforced
- **WHEN** the `encrypted_data` field exceeds 1MB (base64-encoded)
- **THEN** the response status is 400 Bad Request

### Requirement: Retrieve a text secret
The server SHALL accept `POST /api/v1/secrets/{publicID}` with an `X-Retrieval-Token` header. The server SHALL hash the provided token and compare it against the stored hash using constant-time comparison. On match, the server SHALL return the secret's `nonce`, `encrypted_data`, `secret_type`, `burn_after_read`, and `password_protected` fields.

#### Scenario: Successful retrieval
- **WHEN** a POST request is made to `/api/v1/secrets/{publicID}` with a valid `X-Retrieval-Token`
- **THEN** the response status is 200
- **AND** the body contains `nonce`, `encrypted_data`, `secret_type`, `burn_after_read`, and `password_protected`

#### Scenario: Invalid retrieval token
- **WHEN** the `X-Retrieval-Token` does not match the stored hash
- **THEN** the response status is 403 Forbidden

#### Scenario: Secret not found
- **WHEN** the `publicID` does not exist in the database
- **THEN** the response status is 404 Not Found

#### Scenario: Expired secret not retrievable
- **WHEN** the secret's `expires_at` is in the past
- **THEN** the response status is 404 Not Found

#### Scenario: Missing retrieval token header
- **WHEN** the `X-Retrieval-Token` header is absent
- **THEN** the response status is 400 Bad Request

### Requirement: Burn-after-read
When a secret has `burn_after_read` set to true, the server SHALL delete the secret from the database atomically upon successful retrieval. Only the first retrieval request SHALL receive the data.

#### Scenario: Burn-after-read secret deleted after retrieval
- **WHEN** a burn-after-read secret is retrieved successfully
- **THEN** the secret data is returned
- **AND** the secret is deleted from the database

#### Scenario: Second retrieval of burn-after-read secret fails
- **WHEN** a burn-after-read secret has already been retrieved
- **THEN** the response status is 404 Not Found

### Requirement: Delete a secret
The server SHALL accept `DELETE /api/v1/secrets/{publicID}` with both `X-Retrieval-Token` and `X-Deletion-Token` headers. Both tokens MUST match their stored hashes for the deletion to proceed.

#### Scenario: Successful deletion
- **WHEN** a DELETE request provides valid retrieval and deletion tokens
- **THEN** the secret is deleted from the database
- **AND** the response status is 204 No Content

#### Scenario: Invalid deletion token
- **WHEN** the `X-Deletion-Token` does not match
- **THEN** the response status is 403 Forbidden

#### Scenario: Missing deletion token header
- **WHEN** the `X-Deletion-Token` header is absent
- **THEN** the response status is 400 Bad Request

### Requirement: Expiration time parsing
The server SHALL accept the following expiration strings and convert them to absolute timestamps: `5m` (5 minutes), `10m` (10 minutes), `15m` (15 minutes), `1h` (1 hour), `4h` (4 hours), `12h` (12 hours), `1d` (24 hours), `3d` (72 hours), `7d` (168 hours).

#### Scenario: Valid expiration strings
- **WHEN** the expiration is `"7d"`
- **THEN** the `expires_at` is set to 168 hours from now

#### Scenario: Day-based expiration
- **WHEN** the expiration is `"1d"`
- **THEN** the `expires_at` is set to 24 hours from now
