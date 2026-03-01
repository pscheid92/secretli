## Why

The `internal/server` package has ~235 lines of hand-rolled middleware (recovery, request ID, logging, CORS, security headers, middleware chaining) and manual route registration with per-handler rate limit wrapping. Chi provides all of this out of the box using the standard `net/http` interfaces our handlers already use, so migration is incremental — handler signatures don't change.

## What Changes

- Replace `net/http.ServeMux` with `chi.Router` for route registration
- Replace hand-rolled `chain()` helper, `recoveryMiddleware`, `requestIDMiddleware`, `loggingMiddleware`, `securityHeadersMiddleware`, and `corsMiddleware` with chi's built-in `middleware.*` equivalents
- Use chi route groups to apply rate limits per-group instead of wrapping each handler individually
- Keep `sessionMiddleware` as a custom chi-compatible middleware (chi uses the standard `func(http.Handler) http.Handler` signature)
- Keep `IPRateLimiter` and its `RateLimit` function — adapt signature to chi middleware pattern
- Add `go-chi/chi/v5` and `go-chi/cors` as dependencies

## Capabilities

### New Capabilities

_(none — this is a refactor, not a new feature)_

### Modified Capabilities

- `go-server`: Route registration changes from `mux.HandleFunc("METHOD /path", handler)` to chi's `r.Method("METHOD", "/path", handler)` with grouped middleware. Middleware chain order preserved but expressed via chi's `r.Use()`.

## Impact

- **Code**: `internal/server/server.go`, `internal/server/routes.go`, `internal/server/middleware.go` — significant rewrites. `internal/server/middleware_test.go` — tests updated to use chi router.
- **Handlers**: No changes — all handlers keep their `func(w http.ResponseWriter, r *http.Request)` signature. One exception: handlers currently using `r.PathValue("publicID")` (Go 1.22 ServeMux feature) will switch to `chi.URLParam(r, "publicID")`.
- **Dependencies**: Add `github.com/go-chi/chi/v5`, `github.com/go-chi/cors`. Remove `golang.org/x/time/rate` if rate limiter is also migrated (optional).
- **Tests**: `middleware_test.go` and `routes_test.go` (if any) need updating. Handler tests are unaffected since they don't depend on routing.
