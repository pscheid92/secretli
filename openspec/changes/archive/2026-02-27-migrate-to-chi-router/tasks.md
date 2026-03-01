## 1. Dependencies

- [x] 1.1 Add `github.com/go-chi/chi/v5` and `github.com/go-chi/cors` to `go.mod`

## 2. Middleware migration

- [x] 2.1 Remove `recoveryMiddleware`, `requestIDMiddleware`, `loggingMiddleware`, `corsMiddleware`, `securityHeadersMiddleware`, `chain()`, `responseWriter` struct, and `generateRequestID` from `middleware.go`
- [x] 2.2 Adapt `RateLimit` wrapper from `func(http.HandlerFunc) http.HandlerFunc` to chi-compatible `func(http.Handler) http.Handler`
- [x] 2.3 Keep `sessionMiddleware` as-is (already `func(http.Handler) http.Handler`)

## 3. Router and route registration

- [x] 3.1 Replace `net/http.ServeMux` with `chi.NewRouter()` in `server.go`
- [x] 3.2 Register global middleware via `r.Use()`: chi `middleware.Recoverer`, `middleware.RequestID`, custom slog logger, `securityHeadersMiddleware`, `cors.Handler`, `sessionMiddleware`
- [x] 3.3 Rewrite `routes.go` to use chi route groups with per-group rate limiting (auth: 5/min, create: 10/min, retrieve: 30/min, delete: 30/min)
- [x] 3.4 Mount SPA catch-all handler on chi router

## 4. Handler updates

- [x] 4.1 Replace `r.PathValue("publicID")` with `chi.URLParam(r, "publicID")` in `secret.go` (3 call sites) and `file.go` (1 call site)

## 5. Test updates

- [x] 5.1 Update `middleware_test.go` to use chi router instead of raw `http.ServeMux`
- [x] 5.2 Update handler tests that rely on path parameters to use `chi.RouteContext` for URL param injection

## 6. Verification

- [x] 6.1 `go build ./...` passes
- [x] 6.2 `go test ./...` passes
- [x] 6.3 Manual smoke test: health, create secret, retrieve secret, CORS preflight
