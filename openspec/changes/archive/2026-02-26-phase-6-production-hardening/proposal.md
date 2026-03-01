## Why

The core functionality (secret CRUD, file upload/download, auth, user history) is complete but the server lacks production-readiness concerns. Expired secrets and sessions accumulate without cleanup, there's no rate limiting to prevent abuse, no security headers, no CORS support despite the config field existing, and no graceful resource teardown. These are required before any real deployment.

## What Changes

- **Cleanup worker**: Background goroutine running on `CLEANUP_INTERVAL` that calls `DeleteExpired()` for secrets and `DeleteExpiredSessions()` for sessions, then deletes orphaned S3 objects for file-type secrets
- **Rate limiting**: In-memory per-IP token bucket rate limiting via `golang.org/x/time/rate` with different limits per endpoint category (secret creation: 10/min, retrieval: 30/min, auth: 5/min, file upload: 5/min)
- **Security headers**: `X-Content-Type-Options`, `X-Frame-Options`, `Content-Security-Policy`, `Referrer-Policy` on all responses
- **CORS middleware**: Use the existing `AllowedOrigins` config to set `Access-Control-Allow-*` headers, handle preflight `OPTIONS` requests
- **Graceful shutdown enhancement**: Shut down the cleanup worker cleanly when the server receives SIGINT/SIGTERM

## Capabilities

### New Capabilities
- `cleanup-worker`: Background goroutine for periodic expired secret/session deletion and S3 object cleanup
- `rate-limiting`: Per-IP token bucket rate limiting middleware with configurable limits per route category
- `security-headers`: HTTP security header middleware (CSP, X-Frame-Options, etc.)
- `cors`: CORS middleware using the existing `AllowedOrigins` config field

### Modified Capabilities
- `go-server`: Server startup now launches cleanup worker and shuts it down gracefully
- `config`: No new env vars needed — `CLEANUP_INTERVAL`, `ALLOWED_ORIGINS` already exist

## Impact

- **New files**: `internal/cleanup/worker.go`, rate limit and security header middleware additions to `internal/server/middleware.go`
- **Modified files**: `cmd/server.go` (start/stop cleanup worker), `internal/server/middleware.go` (add CORS, rate limit, security headers), `internal/server/routes.go` (wire rate limit middleware per route group)
- **New dependency**: `golang.org/x/time/rate` (already in DESIGN.md, needs `go get`)
- **No breaking changes**: All additions are additive middleware and a background worker
