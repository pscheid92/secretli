## Context

The server package (`internal/server/`) currently uses Go's stdlib `net/http.ServeMux` for routing and ~235 lines of hand-rolled middleware for recovery, request IDs, logging, CORS, security headers, and middleware chaining. Rate limiting is applied per-handler via wrapper functions. This works but is boilerplate-heavy and differs from idiomatic Go HTTP patterns that teams expect.

Chi is a lightweight router built on `net/http` — it uses the same `http.Handler` and `http.HandlerFunc` interfaces, so existing handler code is unaffected. Chi also provides battle-tested middleware for the exact patterns we've implemented manually.

## Goals / Non-Goals

**Goals:**
- Replace `net/http.ServeMux` with `chi.Router`
- Replace hand-rolled middleware with chi built-ins where equivalent
- Use route groups to organize rate limiting by endpoint category
- Reduce code in `server.go`, `routes.go`, and `middleware.go`
- Keep all existing behavior identical (same routes, same middleware order, same responses)

**Non-Goals:**
- Changing handler signatures or return types (no `echo.Context`-style migration)
- Replacing the custom `IPRateLimiter` — it works fine, just needs its wrapper adapted to `func(http.Handler) http.Handler`
- Adding new middleware or features (e.g., request logging format changes)
- Migrating the SPA handler — it stays as-is, mounted on chi's catch-all

## Decisions

### 1. Use chi built-in middleware for recovery, request ID, and logging

**Choice:** Replace `recoveryMiddleware`, `requestIDMiddleware`, and `loggingMiddleware` with `chi/middleware.Recoverer`, `chi/middleware.RequestID`, and a custom logger using chi's `middleware.RequestLogger` interface.

**Rationale:** Recovery and request ID are direct replacements. For logging, chi's `RequestLogger` gives us the same structured output with `slog` but handles the response-writer wrapping internally. Our current custom `responseWriter` wrapper (for capturing status codes) is eliminated.

**Alternative considered:** Keep all custom middleware, only use chi for routing. Rejected because the whole point is reducing boilerplate.

### 2. Use go-chi/cors for CORS middleware

**Choice:** Replace the 35-line hand-rolled `corsMiddleware` with `github.com/go-chi/cors`.

**Rationale:** Handles preflight, allowed methods/headers, credentials, and max-age with declarative config. Our current implementation is correct but fragile to extend.

### 3. Keep security headers as a simple custom middleware

**Choice:** Keep `securityHeadersMiddleware` as custom code, registered via `r.Use()`.

**Rationale:** It's 8 lines, chi doesn't have a built-in for it, and adding a dependency for header-setting is overkill.

### 4. Adapt rate limiter to chi middleware pattern

**Choice:** Change `RateLimit` from `func(http.HandlerFunc) http.HandlerFunc` to `func(http.Handler) http.Handler` so it works with `r.Use()` in route groups.

**Rationale:** Chi middleware uses the standard `func(http.Handler) http.Handler` signature. Our current per-handler wrapping (`createLimit(sh.CreateSecret)`) becomes a group-level `r.Use(RateLimit(rl, 10))`.

### 5. Use chi.URLParam instead of r.PathValue

**Choice:** Replace `r.PathValue("publicID")` with `chi.URLParam(r, "publicID")` in all handlers (4 call sites).

**Rationale:** Chi uses its own context-based URL parameter extraction. `r.PathValue` is a Go 1.22 `ServeMux` feature that won't work with chi's router.

### 6. Keep sessionMiddleware as custom code

**Choice:** Keep the session middleware that populates user context. It already has the `func(http.Handler) http.Handler` signature chi expects.

**Rationale:** This is domain-specific logic (cookie lookup, DB query, context injection). No library replaces it.

## Risks / Trade-offs

- **[Middleware order changes]** → Verify by comparing logged request flow before and after. Chi's `r.Use()` applies middleware in declaration order, which is intuitive but different from our reversed `chain()` helper.
- **[PathValue migration]** → Only 4 call sites, straightforward find-and-replace. Handler tests already pass the param via the URL, so they need updating to use chi's test helpers or `chi.RouteContext`.
- **[Test changes]** → `middleware_test.go` tests will need to use a chi router instead of raw `http.ServeMux`. The tests that check middleware behavior (recovery, rate limiting, CORS) need a real chi router to work correctly. Risk is low since behavior is unchanged.
