## Requirements

### Requirement: User secrets history endpoint
The server SHALL accept `GET /api/v1/user/secrets?page=1&per_page=20` with a valid session cookie. The server SHALL return a paginated list of secrets created by the authenticated user, ordered by `created_at` descending. Each entry SHALL include `public_id`, `label`, `secret_type`, `burn_after_read`, `expires_at`, `created_at`, `retrieved_at`, and `password_protected`. The response SHALL NOT include `encrypted_data`, `nonce`, or token fields.

#### Scenario: Authenticated user with secrets
- **WHEN** a GET request is made with a valid session and the user has secrets
- **THEN** the response status is 200
- **AND** the body contains `{ secrets: [...], page, per_page, total }`

#### Scenario: Authenticated user with no secrets
- **WHEN** a GET request is made and the user has no secrets
- **THEN** the response status is 200 with `{ secrets: [], page: 1, per_page: 20, total: 0 }`

#### Scenario: Pagination
- **WHEN** the user has 25 secrets and requests `?page=2&per_page=20`
- **THEN** the response contains 5 secrets and `total: 25`

#### Scenario: Unauthenticated request
- **WHEN** a GET request is made without a valid session
- **THEN** the response status is 401 Unauthorized

#### Scenario: Default pagination
- **WHEN** `page` and `per_page` are omitted
- **THEN** defaults are `page=1` and `per_page=20`

### Requirement: User secrets join table
The `user_secrets` table SHALL have columns: `user_id` (BIGINT, FK to users ON DELETE CASCADE), `secret_id` (BIGINT, FK to secrets ON DELETE CASCADE), `label` (TEXT NOT NULL DEFAULT ''), `created_at` (TIMESTAMPTZ NOT NULL DEFAULT NOW()), with a composite primary key `(user_id, secret_id)`.

#### Scenario: User secrets table schema
- **WHEN** the user_secrets migration is applied
- **THEN** the table has the specified columns, constraints, and cascade behavior

### Requirement: Link secret to user on creation
When an authenticated user creates a secret (text or file), the server SHALL insert a row into `user_secrets` linking the user to the created secret. The `label` field SHALL be taken from the request body if provided, defaulting to empty string.

#### Scenario: Authenticated user creates text secret
- **WHEN** an authenticated user creates a text secret with `label: "API keys"`
- **THEN** a `user_secrets` row is created with the user's ID, the secret's ID, and label `"API keys"`

#### Scenario: Authenticated user creates file secret
- **WHEN** an authenticated user uploads an encrypted file with `label: "Contract PDF"`
- **THEN** a `user_secrets` row is created linking the user to the file secret

#### Scenario: Anonymous user creates secret
- **WHEN** an unauthenticated user creates a secret
- **THEN** no `user_secrets` row is created

#### Scenario: Label defaults to empty
- **WHEN** an authenticated user creates a secret without providing a `label`
- **THEN** the `user_secrets.label` column is set to empty string
