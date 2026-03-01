## Context

Phases 1–5 delivered the full feature set: text/file secret sharing with zero-knowledge encryption, authentication, and user history. The server currently starts, serves requests, and shuts down gracefully (SIGINT/SIGTERM with 30s drain). However, it lacks production-readiness cross-cutting concerns: expired data accumulates indefinitely, there's no abuse prevention, no security headers, and no CORS support despite the config field already existing.

The building blocks are already in place:
- `SecretRepo.DeleteExpired()` returns expired secret IDs and their S3 storage keys
- `SessionRepo.DeleteExpiredSessions()` exists but is never called
- `FileStore.Delete()` can remove S3 objects
- `Config.CleanupInterval` and `Config.AllowedOrigins` are parsed but unused

## Goals / Non-Goals

**Goals:**
- Automatically clean up expired secrets (DB rows + S3 objects) and expired sessions on a configurable interval
- Rate limit API endpoints per IP to prevent abuse, with different limits per endpoint category
- Add standard security headers to all responses
- Support CORS for cross-origin deployments using the existing `AllowedOrigins` config
- Shut down the cleanup worker gracefully alongside the HTTP server

**Non-Goals:**
- Distributed rate limiting (Redis-backed) — in-memory is sufficient for single-instance deployments
- WAF-level protections or bot detection
- Dynamic rate limit configuration (requires restart to change)
- Metrics/observability instrumentation (future phase)

## Decisions

### 1. Cleanup worker as a standalone goroutine with context cancellation

The cleanup worker runs as a single goroutine started from `cmd/server.go`, controlled by a `context.Context` derived from the shutdown signal. Uses `time.Ticker` at `Config.CleanupInterval`.

**Why not a cron library?** Overkill for a single periodic task. A ticker + select loop is simpler, has no dependencies, and is easy to test.

**Why not run cleanup in a separate process?** The DESIGN.md specifies a single binary deployment. The cleanup query uses `FOR UPDATE SKIP LOCKED`, so multiple replicas are safe.

### 2. Rate limiting with `golang.org/x/time/rate` per IP

Each IP gets its own `rate.Limiter` stored in a `sync.Map`. Limiters are lazily created on first request. A background sweep in the cleanup worker removes stale entries older than 10 minutes to prevent memory growth.

**Rate limits (from DESIGN.md):**
- Secret creation: 10/min
- Secret retrieval: 30/min
- Auth endpoints: 5/min
- File upload: 5/min
- General/other: 60/min

**Why per-IP and not per-user?** Unauthenticated endpoints (secret create/retrieve) are the primary abuse vector. Per-IP covers both authenticated and anonymous users uniformly.

**Why `sync.Map` over `map` + `sync.RWMutex`?** `sync.Map` is optimized for the read-heavy, append-mostly access pattern of rate limiters.

### 3. Rate limiting applied as route-group middleware, not global

Rather than a single global rate limiter, rate limiting is applied as middleware wrapping specific route groups. Each handler function is wrapped with the appropriate rate limit tier. This avoids penalizing health checks and static file serving.

### 4. Security headers as a single middleware

All security headers applied in one middleware early in the chain (after recovery, before routing). Headers match DESIGN.md exactly:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`
- `Referrer-Policy: no-referrer`

### 5. CORS as stdlib middleware (no third-party library)

CORS logic is straightforward: check `Origin` against allowed list, set headers, handle `OPTIONS` preflight. No need for a library. If `AllowedOrigins` is empty, CORS middleware is a no-op (same-origin only).

## Risks / Trade-offs

- **In-memory rate limiting resets on restart** → Acceptable for the threat model. Restarts are infrequent and the window is small.
- **Per-IP rate limiting can be bypassed by distributed attackers** → Out of scope; a WAF or CDN handles this in production.
- **`sync.Map` memory for rate limiters grows with unique IPs** → Mitigated by periodic stale entry cleanup (10min TTL) in the cleanup worker.
- **Cleanup worker single-goroutine could be slow with many expired secrets** → The `LIMIT 500` + `SKIP LOCKED` query bounds each cycle. Multiple replicas can run cleanup concurrently without conflict.
