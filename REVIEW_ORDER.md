# Backend Code Review Order

Recommended reading order for reviewing the Go backend. Follows the dependency
graph bottom-up: domain contracts first, then platform utilities, then adapters,
then wiring.

Tests are listed alongside their source. Review inline or defer to a second pass.

---

## 1. Entry Point

_How the application boots — read this first to see the big picture, then
come back after reviewing the pieces._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 1 | `main.go` | 13 | Minimal entry point, delegates to `cmd.Run` |
| 2 | `cmd/server.go` | ~120 | Composition root: creates all dependencies (DB pool, S3 client, repo, metrics), wires them into `httpserver.New()`, sets up correlated JSON logger, runs graceful shutdown |

**Key questions:** Is the setup sequence clear? Are resources cleaned up on
shutdown? Does `runGracefulShutdown` handle all edge cases (signal during
startup, worker error)?

---

## 2. Configuration

_What knobs exist and how env vars are parsed._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 3 | `internal/platform/config/config.go` | 31 | Struct tags with `required`, defaults |
| 4 | `internal/platform/config/config_test.go` | 91 | Required var validation, defaults, invalid input |

**Key questions:** Are all env vars documented in `.env.example`? Are defaults
safe for production?

---

## 3. Domain Layer

_The core abstractions — interfaces, errors, and types that all other packages
depend on. Small but foundational._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 5 | `internal/domain/errors.go` | 9 | Sentinel errors: `ErrNotFound`, `ErrDuplicate` |
| 6 | `internal/domain/repo.go` | 17 | `SecretRepo` interface — the persistence contract |
| 7 | `internal/domain/filestore.go` | 13 | `FileStore` interface — the object storage contract |
| 8 | `internal/domain/secret.go` | ~65 | `Secret` struct, request types, response types, validation tags |

**Key questions:** Are the interfaces minimal? Do they cover all use cases
without leaking implementation details? Do json tags and validation rules match
the API contract? Could any fields leak sensitive data?

---

## 4. Database Schema

_What's persisted — read migrations in order._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 9 | `internal/adapter/postgres/migrations/001_create_secrets.sql` | 24 | Table structure, indexes, constraints |
| 10 | `internal/adapter/postgres/migrations/002_create_users.sql` | 12 | _(dead — dropped in 005)_ |
| 11 | `internal/adapter/postgres/migrations/003_create_sessions.sql` | 13 | _(dead — dropped in 005)_ |
| 12 | `internal/adapter/postgres/migrations/004_create_user_secrets.sql` | 11 | _(dead — dropped in 005)_ |
| 13 | `internal/adapter/postgres/migrations/005_drop_auth_tables.sql` | 7 | Confirms auth removal |
| 14 | `internal/adapter/postgres/migrations/006_uuidv7_secrets_id.sql` | 11 | PK migration to UUIDv7 |

**Key questions:** Are indexes sufficient for query patterns? Could migrations
002–004 be squashed since they're immediately dropped?

---

## 5. Platform Utilities

_Shared infrastructure with no internal dependencies._

### Crypto

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 15 | `internal/platform/crypto/hash.go` | 24 | SHA256 hashing, constant-time comparison |
| 16 | `internal/platform/crypto/hash_test.go` | 66 | Correctness, edge cases |

**Key questions:** Is `subtle.ConstantTimeCompare` used? Is SHA256 over
unsalted tokens appropriate for this threat model?

### Structured Errors

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 17 | `internal/platform/errors/errors.go` | ~95 | `Error` type with `ErrorType`, `HTTPStatus()`, `ToResponse()`, `AsAppError()` |
| 18 | `internal/platform/errors/errors_test.go` | ~100 | Constructors, status mapping, unwrap, response formatting |

**Key questions:** Are all error types covered? Is `AsAppError` safe for
unexpected error types?

### Correlation ID

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 19 | `internal/platform/correlation/slog.go` | ~40 | slog handler wrapper that injects `request_id` from chi middleware into every log record |

**Key questions:** Does it correctly propagate `WithAttrs`/`WithGroup`?

---

## 6. Postgres Adapter

_SQL queries, generated code, and the Postgres repository implementation._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 20 | `sqlc.yaml` | ~20 | Code generation config, type overrides |
| 21 | `internal/adapter/postgres/queries/secrets.sql` | 37 | Named queries — source of truth for DB operations |
| 22 | `internal/adapter/postgres/dbsqlc/models.go` | 28 | Generated model (compare with `domain.Secret`) |
| 23 | `internal/adapter/postgres/dbsqlc/secrets.sql.go` | 207 | Generated query functions |
| 24 | `internal/adapter/postgres/dbsqlc/db.go` | 32 | Generated DB interface |
| 25 | `internal/adapter/postgres/secret_repo.go` | ~185 | `SecretRepo` implementation — `pgconn.PgError` for duplicates, `domain.ErrNotFound` mapping, pgtype helpers |
| 26 | `internal/adapter/postgres/postgres.go` | 24 | Connection pool setup |
| 27 | `internal/adapter/postgres/migrations.go` | 53 | Migration runner with advisory locking |
| 28 | `internal/adapter/postgres/testhelper_test.go` | 85 | TestContainers setup |
| 29 | `internal/adapter/postgres/secret_repo_test.go` | ~210 | Integration tests against real Postgres |

**Key questions:** Is the burn-after-read retrieval truly atomic (`DELETE ...
RETURNING`)? Does `isDuplicateKeyError` use `pgconn.PgError` properly? Are
pgtype conversion helpers correct for nullable fields?

---

## 7. S3 Adapter

_Encrypted file blob storage via S3/MinIO._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 30 | `internal/adapter/s3/client.go` | ~60 | `Client` with Put/Get/Delete, error wrapping |
| 31 | `internal/adapter/s3/client_test.go` | ~240 | Integration tests with MinIO TestContainer |

**Key questions:** Are S3 errors properly propagated? Cleanup on partial upload
failure? Bucket existence check on init?

---

## 8. HTTP Server Adapter

_Core business logic and HTTP wiring — the most important section to review
carefully._

### Error-returning handler pattern

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 32 | `internal/adapter/httpserver/handler.go` | ~45 | `HandlerFunc` type, centralized `handleError`, `writeJSON`, `validationError` |

### Handlers

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 33 | `internal/adapter/httpserver/validate.go` | ~55 | Request validation, custom `expiration` validator, field error formatting |
| 34 | `internal/adapter/httpserver/secret_handler.go` | ~240 | Create, retrieve (with burn), delete, metadata — returns `*apperrors.Error` |
| 35 | `internal/adapter/httpserver/file_handler.go` | ~195 | Multipart upload, file download, size limits — returns `*apperrors.Error` |
| 36 | `internal/adapter/httpserver/health.go` | ~30 | Liveness vs readiness semantics |

### Server & routing

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 37 | `internal/adapter/httpserver/server.go` | ~110 | Server construction, SPA serving, timeouts — receives `SecretRepo` and `FileStore` via DI |
| 38 | `internal/adapter/httpserver/routes.go` | ~65 | Route definitions with `r.Method("POST", "/", HandlerFunc(...))`, rate limit config |
| 39 | `internal/adapter/httpserver/middleware.go` | ~65 | Security headers, request logging (uses `slog.InfoContext` for auto request_id), request ID propagation |

### Tests

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 40 | `internal/adapter/httpserver/secret_handler_test.go` | ~540 | Handler unit tests via `HandlerFunc.ServeHTTP` with mock repo |
| 41 | `internal/adapter/httpserver/file_handler_test.go` | ~585 | File handler tests with mock file store |
| 42 | `internal/adapter/httpserver/health_test.go` | ~80 | Health endpoint tests |
| 43 | `internal/adapter/httpserver/middleware_test.go` | ~185 | Middleware, SPA handler, parseOrigins tests |

**Key questions:** Is input validation thorough? Are error responses consistent
and not leaking internals? Is the retrieval token checked before any data is
returned? Are all error paths tested? Does `handleError` log internal errors
but hide details from clients?

---

## 9. Cleanup Worker

_Background garbage collection of expired secrets._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 44 | `internal/cleanup/worker.go` | 83 | Ticker loop, S3 + DB cleanup coordination, context-aware logging |
| 45 | `internal/cleanup/worker_test.go` | 132 | Cycle tests, error handling, context cancellation |

**Key questions:** What happens if S3 delete succeeds but DB delete fails (or
vice versa)? Is the cleanup interval configurable? Is logging proportionate?

---

## 10. Metrics Adapter

_Prometheus instrumentation — review last since it's cross-cutting._

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 46 | `internal/adapter/metrics/registry.go` | 22 | Registry setup |
| 47 | `internal/adapter/metrics/secret.go` | 47 | Business counters (created, retrieved, deleted, cleanup errors) |
| 48 | `internal/adapter/metrics/http.go` | 84 | Request duration/count middleware, in-flight gauge |

**Key questions:** Are metric labels bounded (no high-cardinality IDs)? Is
`/metrics` protected or rate-limited?

---

## 11. Embedded Assets

| # | File | Lines | What to look for |
|---|------|------:|------------------|
| 49 | `web/embed.go` | 6 | `//go:embed` directive for frontend dist |

---

## Import Graph (clean DAG, no cycles)

```
cmd/server.go → platform/{config,correlation}, adapter/{postgres,s3,metrics,httpserver}, cleanup, domain
adapter/httpserver → domain, platform/{config,crypto,errors}, adapter/metrics
adapter/postgres  → domain, adapter/postgres/dbsqlc
adapter/s3        → (no internal deps)
adapter/metrics   → (no internal deps)
cleanup           → domain, adapter/metrics
domain            → (no internal deps)
platform/*        → (no internal deps)
```

---

## Supplementary Files

Not Go source, but relevant to the backend review:

| File | Why |
|------|-----|
| `.env.example` | Documents all config knobs |
| `Makefile` | Build/test/dev commands |
| `.air.toml` | Hot-reload config |
| `.golangci.yml` | Linting rules |
| `Dockerfile` | Multi-stage build, final image |
| `.github/workflows/ci.yml` | CI pipeline |
| `docker/docker-compose.yml` | Service dependencies |
