## Why

Secretli is being rebooted from two disconnected repos (Go CLI + Vue 3 frontend) into a single full-stack monorepo. Phase 1 establishes the project scaffold: the foundational directory structure, build tooling, and development workflow that all subsequent phases build on. Without this, there is no codebase to work in.

## What Changes

- Initialize Go module (`github.com/secretli/secretli`) with Go 1.23 and stdlib `net/http` server
- Create React 19 + TypeScript 5 + Vite 6 + Tailwind CSS 4 frontend app under `web/frontend/`
- Set up `embed.go` to embed the React build output into the Go binary via `embed.FS`
- Implement SPA handler that serves the embedded frontend and falls back to `index.html` for client-side routing
- Implement health check endpoints (`GET /api/v1/health/live`, `GET /api/v1/health/ready`)
- Create basic middleware chain: recovery, request ID, structured logging (`log/slog`)
- Set up configuration loading from environment variables (`internal/config/`)
- Create Makefile with `dev`, `build`, and `test` targets
- Configure Air (`.air.toml`) for Go hot-reload during development
- Configure Vite proxy to forward `/api` requests to the Go backend in development
- Create `docker-compose.yml` for local dev infrastructure (PostgreSQL + MinIO)
- Set up a minimal React app with React Router 7, a Layout component, and placeholder pages

## Capabilities

### New Capabilities
- `go-server`: HTTP server with stdlib routing, SPA handler, health endpoints, middleware chain, and embedded frontend serving
- `react-app`: React 19 + TypeScript + Vite + Tailwind frontend with routing and layout shell
- `dev-workflow`: Makefile, Air hot-reload, Vite proxy, docker-compose for local Postgres + MinIO
- `config`: Environment variable-based configuration loading for all server settings

### Modified Capabilities

## Impact

- Creates the entire project directory structure as defined in DESIGN.md section 6
- Establishes Go dependencies: initially just stdlib (pgx, minio-go, goose added in later phases)
- Establishes npm dependencies: react, react-dom, react-router, @tanstack/react-query, tailwindcss
- Sets the development workflow pattern (Air + Vite) used for all subsequent phases
- The embedded frontend approach means production builds require `npm run build` before `go build`
