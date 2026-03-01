## Context

Phase 1 delivered a running Go server with embedded React frontend, health endpoints, and middleware. The server currently has no data persistence. This phase adds PostgreSQL for storing encrypted secrets and implements the full text secret CRUD lifecycle.

The encryption protocol is entirely client-side — the server only stores opaque ciphertext, hashed tokens, and metadata. The server never sees plaintext.

Existing code: `internal/config/` (has `DatabaseURL` field ready), `internal/server/` (routing, middleware), `internal/handler/` (health endpoints).

## Goals / Non-Goals

**Goals:**
- PostgreSQL integration with connection pooling (`pgxpool`)
- Database migrations via goose (embedded SQL files)
- Full secret CRUD: create, retrieve, delete text secrets
- Token hashing and verification (SHA-256, constant-time)
- Expiration parsing and burn-after-read
- `migrate` subcommand on the binary
- Database-aware readiness check

**Non-Goals:**
- File upload/download (Phase 4)
- User authentication or user_secrets table (Phase 5)
- S3/MinIO integration (Phase 4)
- Cleanup worker for expired secrets (Phase 6)
- Rate limiting (Phase 6)

## Decisions

### 1. Repository pattern with interfaces
The `SecretRepo` is defined as an interface in `internal/store/`. This enables unit-testing handlers with mock implementations without needing a real database. The concrete implementation uses `pgxpool.Pool`.

**Alternative considered:** Direct pgxpool calls in handlers. Rejected — makes handlers untestable without a database.

### 2. Goose with embedded SQL migrations
Using `goose/v3` with `embed.FS` to embed migration SQL files into the binary. This means `./secretli migrate` works without needing the migration files on disk. The `migrations/` directory contains `.sql` files that are embedded at compile time.

**Alternative considered:** golang-migrate. Rejected — goose's embed support is simpler and it handles up/down migrations well.

### 3. Only create the `secrets` table in this phase
The `users`, `sessions`, and `user_secrets` tables are deferred to Phase 5. The `secrets` table has no foreign key to `users` — the optional `created_by` link will be added via a migration in Phase 5.

### 4. SHA-256 for token hashing, not bcrypt
Retrieval and deletion tokens are 16 bytes of cryptographic randomness. SHA-256 is sufficient because there's no dictionary to attack. bcrypt would add ~100ms latency to every retrieval for no security benefit. Comparison uses `crypto/subtle.ConstantTimeCompare`.

### 5. POST for retrieval (not GET)
Retrieval has side effects: it may delete the secret (burn-after-read) and sets `retrieved_at`. Using POST makes this semantically correct and avoids caching issues.

### 6. Expiration stored as absolute timestamp
The handler converts relative expiration strings (e.g., `"7d"`) to absolute `time.Time` using `time.Now().Add(duration)` before storing. This simplifies cleanup queries (`WHERE expires_at < NOW()`).

### 7. Migrate command integrated into main binary
Rather than a separate migration binary, the main binary accepts a `migrate` argument: `./secretli migrate`. This is checked in `cmd/server.go` via `os.Args`. Keeps deployment simple — one binary does everything.

**Alternative considered:** Using cobra for subcommands. Rejected — overkill for two code paths (`serve` default, `migrate`). Can add later if needed.

### 8. Database connection validation at startup
The server validates the database connection at startup by calling `pool.Ping()`. If the database is unreachable, the server exits immediately with a clear error. The readiness endpoint also pings the database.

## Risks / Trade-offs

- **Risk:** `pgxpool` connection exhaustion under load. → **Mitigation:** Default pool size is `max(4, numCPU)` which is sensible. Can tune via `DATABASE_URL` query params (`pool_max_conns`).

- **Risk:** Burn-after-read race condition — two concurrent retrieval requests for the same secret could both succeed. → **Mitigation:** Use `DELETE ... RETURNING` in a single query for burn-after-read secrets, making it atomic. Only the first request gets data; the second gets a 404.

- **Risk:** Migration failures during deploy. → **Mitigation:** Goose runs migrations in a transaction. Failed migrations roll back. The `migrate` command is run before the server starts (in Helm, via a pre-upgrade job).

- **Trade-off:** No cleanup worker yet — expired secrets stay in the database until Phase 6. This is acceptable for development; the `expires_at` check in the retrieval handler prevents access to expired secrets.
