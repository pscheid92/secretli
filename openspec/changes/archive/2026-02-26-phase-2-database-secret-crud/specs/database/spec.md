## ADDED Requirements

### Requirement: Secrets table schema
The database SHALL have a `secrets` table matching the schema in DESIGN.md section 4: `id` (bigserial PK), `public_id` (unique text), `retrieval_token_hash` (text), `deletion_token_hash` (text), `encrypted_data` (nullable text), `nonce` (text), `secret_type` (text, default 'text'), `storage_key` (nullable text), `encrypted_filename` (nullable text), `encrypted_size` (nullable bigint), `burn_after_read` (boolean), `password_protected` (boolean), `expires_at` (timestamptz), `created_at` (timestamptz), `retrieved_at` (nullable timestamptz). Indexes on `public_id` and `expires_at`.

#### Scenario: Migration creates secrets table
- **WHEN** `./secretli migrate` is run against an empty database
- **THEN** the `secrets` table exists with all columns and indexes

#### Scenario: Migration is idempotent
- **WHEN** `./secretli migrate` is run twice
- **THEN** the second run succeeds without errors

### Requirement: Database connection pool
The server SHALL connect to PostgreSQL using `pgxpool` with the URL from the `DATABASE_URL` environment variable. The pool SHALL be initialized at server startup and closed on shutdown.

#### Scenario: Successful connection
- **WHEN** `DATABASE_URL` points to a valid PostgreSQL instance
- **THEN** the server starts and the pool is available

#### Scenario: Database unreachable at startup
- **WHEN** `DATABASE_URL` points to an unreachable host
- **THEN** the server exits with an error message

### Requirement: Migrate subcommand
The binary SHALL accept `./secretli migrate` to run all pending database migrations using goose. Migration SQL files SHALL be embedded in the binary.

#### Scenario: Run pending migrations
- **WHEN** `./secretli migrate` is run
- **THEN** all pending migrations are applied
- **AND** the process exits with code 0

### Requirement: Secret repository CRUD operations
The store layer SHALL provide: `Create(ctx, secret)`, `GetByPublicID(ctx, publicID)`, `Delete(ctx, publicID)`, and `DeleteExpired(ctx)` operations on the `secrets` table.

#### Scenario: Create and retrieve round-trip
- **WHEN** a secret is created via `Create`
- **THEN** it can be retrieved via `GetByPublicID` with matching data

#### Scenario: Delete removes the secret
- **WHEN** `Delete` is called with a valid `publicID`
- **THEN** subsequent `GetByPublicID` returns not found

#### Scenario: DeleteExpired removes only expired secrets
- **WHEN** `DeleteExpired` is called
- **THEN** secrets with `expires_at` in the past are deleted
- **AND** secrets with `expires_at` in the future are retained
