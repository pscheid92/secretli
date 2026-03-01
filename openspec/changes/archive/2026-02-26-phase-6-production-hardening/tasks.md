## 1. Security Headers Middleware

- [x] 1.1 Add `SecurityHeaders` middleware to `internal/server/middleware.go` that sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`, `Referrer-Policy: no-referrer`
- [x] 1.2 Wire security headers middleware into the middleware chain in `internal/server/server.go` (after recovery/request-id/logging, before routing)

## 2. CORS Middleware

- [x] 2.1 Add `CORS` middleware to `internal/server/middleware.go` that checks `Origin` against `AllowedOrigins`, sets `Access-Control-Allow-*` headers for matching origins, handles `OPTIONS` preflight with `204`, is a no-op when `AllowedOrigins` is empty
- [x] 2.2 Wire CORS middleware into the middleware chain (after security headers, before rate limiting)

## 3. Rate Limiting

- [x] 3.1 Add `golang.org/x/time` dependency via `go get`
- [x] 3.2 Create rate limiter infrastructure in `internal/server/middleware.go`: per-IP `rate.Limiter` store using `sync.Map`, lazy limiter creation, `RateLimit` middleware factory that takes requests-per-minute and returns an `http.HandlerFunc` wrapper
- [x] 3.3 Add `CleanupStaleEntries()` method on the rate limiter store that removes entries not accessed in 10+ minutes
- [x] 3.4 Wire rate limiting per route group in `internal/server/routes.go`: secret creation (10/min), secret retrieval (30/min), auth endpoints (5/min), file upload (5/min), delete (30/min). Health and static endpoints are NOT rate limited.

## 4. Cleanup Worker

- [x] 4.1 Create `internal/cleanup/worker.go` with a `Worker` struct that takes context, cleanup interval, secret repo, session repo, and file store. Runs a `time.Ticker` loop that calls `DeleteExpired`, deletes returned S3 objects (logging failures without stopping), and calls `DeleteExpiredSessions`
- [x] 4.2 Add structured logging: log count of deleted secrets/sessions at info level when > 0, skip logging when nothing was cleaned
- [x] 4.3 Add stale rate limiter cleanup call to the worker (call `CleanupStaleEntries` each cycle)

## 5. Server Integration

- [x] 5.1 Update `cmd/server.go` to create and start the cleanup worker goroutine with the server's shutdown context
- [x] 5.2 Ensure server waits for cleanup worker to finish (via `sync.WaitGroup` or channel) before exiting
- [x] 5.3 Update middleware chain order in `internal/server/server.go`: recovery → request ID → logging → security headers → CORS → session

## 6. Verification

- [x] 6.1 Verify the server compiles and starts cleanly with `go build ./...`
- [x] 6.2 Verify security headers are present on responses via curl (requires running server)
- [x] 6.3 Verify rate limiting returns 429 when exceeded (requires running server)
- [x] 6.4 Verify cleanup worker logs startup and runs on interval (requires running server)
