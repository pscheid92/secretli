## Why

Phase 1 established the project scaffold with a running server, but it has no data persistence or business logic. The core value of Secretli is creating, retrieving, and deleting encrypted secrets — this phase delivers that end-to-end for text secrets, making the backend functionally useful for the first time.

## What Changes

- Add PostgreSQL dependency (`pgx/v5` with `pgxpool`) and database migration tool (`goose/v3`)
- Create SQL migration for the `secrets` table with all columns defined in DESIGN.md
- Implement database connection pool setup and secret repository (Create, GetByPublicID, Delete, DeleteExpired)
- Implement SHA-256 token hashing with constant-time comparison for retrieval and deletion tokens
- Add three API endpoints:
  - `POST /api/v1/secrets` — create a text secret
  - `POST /api/v1/secrets/{publicID}` — retrieve a text secret (with `X-Retrieval-Token` header)
  - `DELETE /api/v1/secrets/{publicID}` — delete a secret (with both `X-Retrieval-Token` and `X-Deletion-Token` headers)
- Implement expiration string parsing (`5m`, `10m`, `15m`, `1h`, `4h`, `12h`, `1d`, `3d`, `7d` → absolute timestamps)
- Implement burn-after-read: secret is deleted after first successful retrieval when flag is set
- Add `migrate` subcommand to the binary so `./secretli migrate` runs goose migrations
- Update the readiness health check to verify database connectivity

## Capabilities

### New Capabilities
- `secret-crud`: Create, retrieve, and delete text secrets via the API with token-based authentication
- `database`: PostgreSQL connection pool, migrations, and the secrets table schema
- `token-hashing`: SHA-256 hashing and constant-time verification of retrieval and deletion tokens

### Modified Capabilities
- `go-server`: Readiness endpoint now checks database connectivity; server requires `DATABASE_URL`

## Impact

- New Go dependencies: `github.com/jackc/pgx/v5`, `github.com/pressly/goose/v3`
- `DATABASE_URL` becomes required for the server to start
- `internal/store/`, `internal/crypto/`, `internal/handler/secret.go` are new packages
- `internal/server/routes.go` gains three new route registrations
- `internal/handler/health.go` readiness handler gains a database check
- `migrations/` directory with SQL files is added
- `cmd/server.go` gains a `migrate` code path
