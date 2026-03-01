## 1. Go Module and Entry Point

- [x] 1.1 Initialize Go module (`go mod init github.com/pscheid92/secretli`) with Go 1.23
- [x] 1.2 Create `main.go` entry point that calls a `run()` function in `cmd/server.go`
- [x] 1.3 Create `cmd/server.go` with server startup logic (parse flags, load config, start HTTP server, handle signals for graceful shutdown)

## 2. Configuration

- [x] 2.1 Create `internal/config/config.go` with `Config` struct containing all fields from DESIGN.md section 8 (Port, DatabaseURL, S3Endpoint, S3Bucket, S3AccessKey, S3SecretKey, S3UseSSL, S3Region, MaxFileSize, CleanupInterval, SessionMaxAge, CookieDomain, CookieSecure, AllowedOrigins)
- [x] 2.2 Implement `Load()` function that reads environment variables with sensible defaults
- [x] 2.3 Write tests for config loading with and without env vars set

## 3. HTTP Server and Middleware

- [x] 3.1 Create `internal/server/server.go` with `New(cfg config.Config)` that returns a configured `*http.Server` with timeouts (read: 30s, write: 60s, idle: 120s)
- [x] 3.2 Create `internal/server/middleware.go` with recovery middleware (catches panics, returns 500, logs with slog)
- [x] 3.3 Add request ID middleware (generates UUID or uses existing `X-Request-ID` header, sets on response)
- [x] 3.4 Add structured logging middleware (logs method, path, status, duration_ms, request_id via `log/slog`)
- [x] 3.5 Compose middleware chain in server setup: recovery → request ID → logging

## 4. Health Endpoints and Routing

- [x] 4.1 Create `internal/handler/health.go` with `Liveness` and `Readiness` handlers returning `{"status":"ok"}` with 200
- [x] 4.2 Create `internal/server/routes.go` that registers health routes using Go 1.22+ patterns (`GET /api/v1/health/live`, `GET /api/v1/health/ready`)
- [x] 4.3 Write tests for health endpoints using `httptest`

## 5. SPA Handler and Frontend Embed

- [x] 5.1 Create `web/embed.go` with `//go:embed all:frontend/dist` directive exposing `DistFS`
- [x] 5.2 Create `web/frontend/dist/.gitkeep` so the embed directory exists for builds without a frontend build
- [x] 5.3 Implement SPA handler in `internal/server/server.go`: serve static files from embedded FS, fall back to `index.html` for unknown paths, use `fs.Sub` to strip `frontend/dist` prefix
- [x] 5.4 Register SPA handler as catch-all `GET /` route (after API routes)

## 6. React Frontend Setup

- [x] 6.1 Scaffold React app in `web/frontend/` using Vite with React + TypeScript template
- [x] 6.2 Install dependencies: `react-router` (v7), `@tanstack/react-query` (v5)
- [x] 6.3 Configure Tailwind CSS 4 via the Vite plugin (`@tailwindcss/vite`)
- [x] 6.4 Configure `vite.config.ts` with proxy: `/api` → `http://localhost:8080`
- [x] 6.5 Set up `index.css` with Tailwind import (`@import "tailwindcss"`)

## 7. React App Structure

- [x] 7.1 Create `src/App.tsx` with React Router setup and `QueryClientProvider`
- [x] 7.2 Create `src/components/Layout.tsx` with nav header (links to Share `/`, File `/file`, Login `/login`) and footer
- [x] 7.3 Create placeholder page components: `SharePage.tsx`, `RetrievePage.tsx`, `FilePage.tsx`, `LoginPage.tsx`, `RegisterPage.tsx`, `HistoryPage.tsx`, `NotFoundPage.tsx`
- [x] 7.4 Wire up all routes in App.tsx matching DESIGN.md section 7 routing table
- [x] 7.5 Verify frontend builds successfully with `npm run build`

## 8. Development Workflow

- [x] 8.1 Create `.air.toml` config: watch `.go` and `.sql` files, exclude `web/frontend/`, `tmp/`, `node_modules/`, build command `go build -o ./tmp/secretli .`, run `./tmp/secretli`
- [x] 8.2 Create `Makefile` with targets: `dev` (runs Air + Vite concurrently), `build` (build-frontend then build-go), `build-frontend`, `build-go`, `test`
- [x] 8.3 Create `deploy/docker-compose.yml` with PostgreSQL (port 5432, db: secretli, user: secretli, password: secretli) and MinIO (port 9000/9001, root user: minioadmin, default bucket: secretli)
- [x] 8.4 Add `.gitignore` for Go and Node.js artifacts (tmp/, bin/, node_modules/, dist/, .env)

## 9. Verification

- [x] 9.1 Run `make build` and verify it produces a working binary at `bin/secretli`
- [x] 9.2 Start the binary and verify `curl localhost:8080/api/v1/health/live` returns `{"status":"ok"}`
- [x] 9.3 Start the binary and verify `curl localhost:8080/` returns the React app's index.html
- [x] 9.4 Run `go test ./...` and verify all Go tests pass
