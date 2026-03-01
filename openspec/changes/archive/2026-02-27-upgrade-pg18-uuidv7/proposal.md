## Why

PostgreSQL 18 introduces the native `uuidv7()` function, providing time-ordered, globally unique identifiers without extensions. Upgrading from PG 17 to 18 lets us replace the `BIGSERIAL` internal primary key with UUIDv7 — better for distributed scenarios and avoiding sequential ID enumeration.

## What Changes

- Upgrade PostgreSQL Docker images from `17-alpine` to `18-alpine`
- **BREAKING**: Change `secrets.id` column from `BIGSERIAL` to `UUID DEFAULT uuidv7()`
- Update Go model and sqlc-generated code to use `uuid.UUID` instead of `int64` for the internal ID
- Add sqlc type override mapping PostgreSQL `uuid` to `github.com/google/uuid.UUID`

## Capabilities

### New Capabilities

_None — this is an infrastructure upgrade, not a new feature._

### Modified Capabilities

- `database`: The `secrets.id` column type changes from `bigserial` to `uuid` with `DEFAULT uuidv7()`. The repository layer returns `uuid.UUID` instead of `int64` for secret IDs.

## Impact

- **Docker**: Both compose files (`docker-compose.yml`, `docker-compose.dev.yml`) need image bump
- **Database**: New migration alters the `id` column type; existing data gets new UUIDs (secrets are ephemeral so this is safe)
- **Go dependencies**: `github.com/google/uuid` promoted from indirect to direct dependency
- **Generated code**: sqlc regeneration changes `int64` → `uuid.UUID` across models and query functions
- **Tests**: ID assertion in `secret_repo_test.go` changes from zero-int check to `uuid.Nil` check
