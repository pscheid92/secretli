## 1. Dependencies and Migrations

- [x] 1.1 Add `pgx/v5` and `goose/v3` to `go.mod` (`go get github.com/jackc/pgx/v5 github.com/pressly/goose/v3`)
- [x] 1.2 Create `migrations/001_create_secrets.sql` with the full secrets table schema, indexes, and goose up/down annotations
- [x] 1.3 Create `internal/store/migrations.go` that embeds the `migrations/` directory and exposes a `RunMigrations(ctx, dbURL)` function using goose
- [x] 1.4 Add `migrate` subcommand to `cmd/server.go`: when `os.Args` contains `migrate`, run migrations and exit

## 2. Database Connection

- [x] 2.1 Create `internal/store/postgres.go` with `NewPool(ctx, databaseURL)` that returns a `*pgxpool.Pool`, calls `pool.Ping()` to validate, and returns an error if unreachable
- [x] 2.2 Wire pool creation into `cmd/server.go`: create pool at startup, defer `pool.Close()`, pass pool to handlers/repos
- [x] 2.3 Update `internal/handler/health.go` readiness handler to accept a pool and ping it — return 503 with `{"status":"unavailable"}` if ping fails

## 3. Token Hashing

- [x] 3.1 Create `internal/crypto/hash.go` with `HashToken(token string) string` (SHA-256 of base64url-decoded bytes, hex output) and `VerifyToken(token, storedHash string) bool` (constant-time comparison)
- [x] 3.2 Write tests for `HashToken` (deterministic, different inputs produce different outputs) and `VerifyToken` (valid/invalid cases)

## 4. Domain Model and Repository

- [x] 4.1 Create `internal/model/secret.go` with `Secret` struct matching all columns of the `secrets` table, plus a `CreateSecretRequest` struct for the API input
- [x] 4.2 Create `internal/store/secret_repo.go` with `SecretRepo` interface: `Create(ctx, *model.Secret) error`, `GetByPublicID(ctx, publicID string) (*model.Secret, error)`, `Delete(ctx, publicID string) error`, `DeleteExpired(ctx) (int64, error)`
- [x] 4.3 Create `internal/store/secret_repo_pg.go` implementing `SecretRepo` with pgxpool — `Create` uses INSERT, `GetByPublicID` uses SELECT with `expires_at > NOW()` check, `Delete` uses DELETE, `DeleteExpired` uses `DELETE FROM secrets WHERE expires_at < NOW()`
- [x] 4.4 For burn-after-read retrieval, implement an atomic `GetAndDeleteByPublicID(ctx, publicID string) (*model.Secret, error)` that uses `DELETE ... RETURNING *` to prevent race conditions

## 5. Secret Handlers

- [x] 5.1 Create `internal/handler/secret.go` with a `SecretHandler` struct that holds the `SecretRepo` interface
- [x] 5.2 Implement `CreateSecret` handler: parse JSON body, validate required fields, validate expiration string, validate `encrypted_data` size (< 1MB), hash tokens, convert expiration to absolute timestamp, call repo.Create, return 201 with `expires_at`
- [x] 5.3 Implement expiration parsing helper: map `5m/10m/15m/1h/4h/12h/1d/3d/7d` to `time.Duration`, return error for invalid values
- [x] 5.4 Implement `RetrieveSecret` handler: extract `publicID` from path, extract `X-Retrieval-Token` header, hash token, fetch secret from repo, verify token hash, handle burn-after-read (use `GetAndDeleteByPublicID`), set `retrieved_at` on first retrieval, return secret data
- [x] 5.5 Implement `DeleteSecret` handler: extract `publicID`, extract both token headers, verify both tokens, delete from repo, return 204

## 6. Route Registration

- [x] 6.1 Update `internal/server/routes.go` to register secret routes: `POST /api/v1/secrets`, `POST /api/v1/secrets/{publicID}`, `DELETE /api/v1/secrets/{publicID}`
- [x] 6.2 Update `internal/server/server.go` `New()` to accept a `*pgxpool.Pool` parameter and pass it to handler constructors and the readiness health check

## 7. Tests

- [x] 7.1 Write handler unit tests for `CreateSecret` using a mock `SecretRepo` (success, validation errors, duplicate public_id)
- [x] 7.2 Write handler unit tests for `RetrieveSecret` (success, invalid token, not found, expired, burn-after-read)
- [x] 7.3 Write handler unit tests for `DeleteSecret` (success, invalid tokens, not found)
- [x] 7.4 Write handler unit test for readiness endpoint with database check (healthy, unhealthy)

## 8. Verification

- [ ] 8.1 Start Postgres via `docker compose -f deploy/docker-compose.yml up -d postgres`, run `./bin/secretli migrate`, verify table exists
- [ ] 8.2 Start the server, create a secret via curl, retrieve it, delete it — full lifecycle
- [ ] 8.3 Test burn-after-read: create with `burn_after_read: true`, retrieve once (200), retrieve again (404)
- [ ] 8.4 Test expiration: create with `5m` expiration, verify `expires_at` is ~5 minutes in the future
- [x] 8.5 Run `go test ./...` and verify all tests pass
