## MODIFIED Requirements

### Requirement: Create a text secret
The server SHALL accept `POST /api/v1/secrets` with a JSON body containing `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `encrypted_data`, `expiration`, `burn_after_read`, `password_protected`, and an optional `label`. The server SHALL hash the tokens with SHA-256 before storing. The server SHALL convert the `expiration` string to an absolute timestamp. If the request has an authenticated session, the server SHALL insert a `user_secrets` row linking the user to the secret with the provided label. On success, the server SHALL return `201 Created` with `{"expires_at": "<ISO8601>"}`.

#### Scenario: Successful secret creation
- **WHEN** a POST request is made to `/api/v1/secrets` with valid JSON body
- **THEN** the secret is stored in the database with hashed tokens
- **AND** the response status is 201 with an `expires_at` field

#### Scenario: Authenticated user creates secret with label
- **WHEN** an authenticated user creates a secret with `label: "SSH key"`
- **THEN** the secret is created and a `user_secrets` row links the user to the secret with label `"SSH key"`

#### Scenario: Anonymous user creates secret
- **WHEN** an unauthenticated user creates a secret
- **THEN** the secret is created normally with no `user_secrets` row

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
