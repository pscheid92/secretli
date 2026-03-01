# Backend Code Review Order

Recommended reading order for reviewing the Go backend. Follows the dependency
graph from foundations upward: understand how the app boots, what data looks
like, how it's stored, then how requests are handled.

Each section lists source files first, then their tests. You can review tests
inline or defer them to a second pass.

**Total:** ~2,100 lines of application code, ~2,200 lines of tests.

---

## 1. Entry Point & Bootstrap

_How the application starts, what it wires together._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 1 | `main.go` | 17 | Minimal — just calls cmd |
| 2 | `cmd/server.go` | 112 | Config loading, DB/S3 init, migration, handler & server construction, cleanup worker launch, graceful shutdown |

**Key questions:** Are resources cleaned up on shutdown? Is the dependency
wiring clear?

---

## 2. Configuration

_What knobs exist and how env vars are parsed._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 3 | `internal/config/config.go` | 31 | Struct tags, defaults, validation |
| 4 | `internal/config/config_test.go` | 70 | Edge cases, missing vars |

**Key questions:** Are all env vars documented in `.env.example`? Are defaults
safe for production?

---

## 3. Domain Model

_Core data structures shared across packages._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 5 | `internal/model/secret.go` | 47 | Request/response types, validation tags, field naming |

**Key questions:** Do validation rules match the API contract? Are there any
fields that could leak sensitive data in responses?

---

## 4. Database Schema

_What's actually persisted — read migrations in order._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 6 | `migrations/001_create_secrets.sql` | 24 | Table structure, indexes, constraints |
| 7 | `migrations/002_create_users.sql` | 12 | (Dead code — dropped in 005) |
| 8 | `migrations/003_create_sessions.sql` | 13 | (Dead code — dropped in 005) |
| 9 | `migrations/004_create_user_secrets.sql` | 11 | (Dead code — dropped in 005) |
| 10 | `migrations/005_drop_auth_tables.sql` | 7 | Confirms auth removal |
| 11 | `migrations/006_uuidv7_secrets_id.sql` | 11 | PK migration to UUIDv7 |

**Key questions:** Are indexes sufficient for query patterns? Is the
`expires_at` partial index used by the cleanup query? Could migrations 002-004
be squashed since they're immediately dropped?

---

## 5. Data Access Layer

_SQL queries, generated code, repository interface, and Postgres implementation._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 12 | `sqlc.yaml` | ~20 | Code generation config, type overrides |
| 13 | `internal/store/queries/secrets.sql` | 37 | Named queries — the source of truth for DB operations |
| 14 | `internal/store/dbsqlc/models.go` | 28 | Generated model (compare with domain model) |
| 15 | `internal/store/dbsqlc/secrets.sql.go` | 207 | Generated query functions |
| 16 | `internal/store/dbsqlc/db.go` | 32 | Generated DB interface |
| 17 | `internal/store/secret_repo.go` | 16 | Repository interface — the contract |
| 18 | `internal/store/secret_repo_pg.go` | 199 | Postgres implementation — transaction handling, burn-after-read atomicity |
| 19 | `internal/store/postgres.go` | 24 | Connection pool setup |
| 20 | `internal/store/migrations.go` | 53 | Migration runner (Goose) |
| 21 | `internal/store/testhelper_test.go` | 85 | TestContainers setup |
| 22 | `internal/store/secret_repo_test.go` | 210 | Integration tests against real Postgres |

**Key questions:** Is the burn-after-read retrieval truly atomic (SELECT +
DELETE in one transaction)? Are SQL queries parameterized? Does the repo
interface make testing/mocking easy?

---

## 6. Crypto Utilities

_Token hashing — small but security-critical._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 23 | `internal/crypto/hash.go` | 24 | SHA256 hashing, constant-time comparison |
| 24 | `internal/crypto/hash_test.go` | 66 | Correctness, edge cases |

**Key questions:** Is `subtle.ConstantTimeCompare` used for token verification?
Is the hash algorithm appropriate (SHA256 over unsalted tokens)?

---

## 7. File Storage (S3/MinIO)

_Encrypted file blob storage._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 25 | `internal/storage/s3.go` | 66 | Put/Get/Delete operations, error handling |
| 26 | `internal/storage/s3_test.go` | 239 | Integration tests with MinIO TestContainer |

**Key questions:** Are S3 errors properly propagated? Is there cleanup on
partial upload failure? Content-type handling?

---

## 8. HTTP Handlers

_Core business logic — the most important section to review carefully._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 27 | `internal/handler/validate.go` | 63 | Request parsing, validation error formatting |
| 28 | `internal/handler/secret.go` | 289 | Create, retrieve (with burn), delete, metadata |
| 29 | `internal/handler/file.go` | 209 | Multipart upload, file download, size limits |
| 30 | `internal/handler/health.go` | 35 | Liveness vs readiness semantics |
| 31 | `internal/handler/secret_test.go` | 544 | Handler tests (mock repo) |
| 32 | `internal/handler/file_test.go` | 587 | File handler tests |
| 33 | `internal/handler/health_test.go` | 82 | Health endpoint tests |

**Key questions:** Is input validation thorough (especially expiration bounds,
file sizes)? Are error responses consistent and not leaking internals? Is the
retrieval token checked before any data is returned? Are all error paths
tested?

---

## 9. Server, Routing & Middleware

_How requests reach handlers._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 34 | `internal/server/server.go` | 126 | Server construction, timeouts, SPA static file serving |
| 35 | `internal/server/routes.go` | 70 | Route definitions, rate limit config, middleware stacking |
| 36 | `internal/server/middleware.go` | 66 | Security headers, request logging, request ID |
| 37 | `internal/server/middleware_test.go` | 185 | Middleware tests |

**Key questions:** Are timeouts reasonable? Is CORS configured correctly for
production? Are rate limits appropriate? Does the SPA fallback handle static
assets vs routes correctly?

---

## 10. Background Cleanup Worker

_Expired secret garbage collection._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 38 | `internal/cleanup/worker.go` | 84 | Ticker-based loop, S3 + DB cleanup coordination, graceful stop |
| 39 | `internal/cleanup/worker_test.go` | 132 | Worker tests |

**Key questions:** What happens if S3 delete succeeds but DB delete fails (or
vice versa)? Is the cleanup interval configurable? Does it log enough for
debugging but not too much for production?

---

## 11. Metrics & Observability

_Prometheus instrumentation — review last since it's cross-cutting._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 40 | `internal/metrics/metrics.go` | 22 | Registry setup |
| 41 | `internal/metrics/secret.go` | 47 | Business metric counters (created, retrieved, deleted) |
| 42 | `internal/metrics/http.go` | 84 | HTTP request duration/count middleware |

**Key questions:** Are metric labels bounded (no high-cardinality labels like
IDs)? Is the `/metrics` endpoint protected or rate-limited?

---

## 12. Embedded Assets

_Trivial but worth a glance._

| # | File | Lines | What to look for |
|---|------|------:|-------------------|
| 43 | `web/embed.go` | 6 | `//go:embed` directive for frontend dist |

---

## Supplementary Files

These aren't Go source but are relevant to the backend review:

| File | Why |
|------|-----|
| `.env.example` | Document all config knobs |
| `Makefile` | Build/test/dev commands |
| `.air.toml` | Hot-reload config |
| `.golangci.yml` | Linting rules |
| `Dockerfile` | Multi-stage build, final image |
| `.github/workflows/ci.yml` | CI pipeline |
| `docker/docker-compose.yml` | Service dependencies |
