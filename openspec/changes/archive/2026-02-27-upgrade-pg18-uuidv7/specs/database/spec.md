## MODIFIED Requirements

### Requirement: Secrets table schema
The database SHALL have a `secrets` table with: `id` (uuid PK, default `uuidv7()`), `public_id` (unique text), `retrieval_token_hash` (text), `deletion_token_hash` (text), `encrypted_data` (nullable text), `nonce` (text), `secret_type` (text, default 'text'), `storage_key` (nullable text), `encrypted_filename` (nullable text), `encrypted_size` (nullable bigint), `burn_after_read` (boolean), `password_protected` (boolean), `expires_at` (timestamptz), `created_at` (timestamptz), `retrieved_at` (nullable timestamptz). Indexes on `public_id` and `expires_at`.

#### Scenario: Migration creates secrets table with UUID primary key
- **WHEN** all migrations are run against an empty database
- **THEN** the `secrets` table exists with `id` column of type `uuid`
- **AND** new rows receive a `uuidv7()` default value for `id`

#### Scenario: Migration is idempotent
- **WHEN** `./secretli migrate` is run twice
- **THEN** the second run succeeds without errors

### Requirement: Secret repository CRUD operations
The store layer SHALL provide: `Create(ctx, secret)`, `GetByPublicID(ctx, publicID)`, `Delete(ctx, publicID)`, and `DeleteExpired(ctx)` operations on the `secrets` table. The `Create` operation SHALL populate `secret.ID` with the generated `uuid.UUID` value.

#### Scenario: Create and retrieve round-trip
- **WHEN** a secret is created via `Create`
- **THEN** it can be retrieved via `GetByPublicID` with matching data
- **AND** the returned `ID` field is a valid non-nil UUID

#### Scenario: Delete removes the secret
- **WHEN** `Delete` is called with a valid `publicID`
- **THEN** subsequent `GetByPublicID` returns not found

#### Scenario: DeleteExpired removes only expired secrets
- **WHEN** `DeleteExpired` is called
- **THEN** secrets with `expires_at` in the past are deleted
- **AND** secrets with `expires_at` in the future are retained
