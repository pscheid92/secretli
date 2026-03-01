## Context

Secretli was originally two separate repos: a Go CLI client and a Vue 3 frontend. The backend API server code was not in those repos. We are rebuilding everything as a single full-stack monorepo with a Go backend and React frontend.

Phase 1 establishes the project scaffold — the directory structure, build tooling, dev workflow, and minimal running server. All subsequent phases (database, encryption, file upload, auth, deployment) build on this foundation.

The full system design is documented in `DESIGN.md` at the project root. This phase implements the structural decisions from that document without any business logic.

## Goals / Non-Goals

**Goals:**
- Runnable Go HTTP server that serves embedded React frontend
- Health check endpoints responding correctly
- `make dev` starts both Go (with hot-reload) and Vite dev server
- `make build` produces a single Go binary with the frontend embedded
- docker-compose provides Postgres and MinIO for local development
- Clean project structure that maps to DESIGN.md section 6
- Minimal middleware chain (recovery, request ID, logging)
- React app with routing, layout shell, and placeholder pages

**Non-Goals:**
- Database connectivity or migrations (Phase 2)
- Secret CRUD endpoints or encryption logic (Phases 2-3)
- File upload/download or S3 integration (Phase 4)
- Authentication or user management (Phase 5)
- Rate limiting, security headers, cleanup worker (Phase 6)
- Dockerfile, Helm charts, CI/CD (Phase 7)
- UI polish, dark mode, animations (Phase 8)

## Decisions

### 1. Go module path: `github.com/pscheid92/secretli`
Matches the old project name with a new user path. Even though this is a monorepo reboot, keeping the same org/repo name maintains continuity.

### 2. No CLI framework (no Cobra)
The old CLI used Cobra for subcommands. The new app is a server, not a CLI. A simple `main.go` that calls a `run()` function is sufficient. If we need subcommands later (e.g., `serve`, `migrate`), we can add them with a lightweight flag-based approach or introduce Cobra then.

**Alternative considered:** Using Cobra from the start for `serve` and `migrate` subcommands. Rejected because we don't need `migrate` yet (Phase 2) and adding Cobra later is trivial.

### 3. Frontend at `web/frontend/` with embed at `web/embed.go`
The `web/` package owns the embedded filesystem. `embed.go` uses `//go:embed frontend/dist/*` and exposes a `DistFS` variable. The server uses `fs.Sub(DistFS, "frontend/dist")` to strip the prefix.

**Alternative considered:** Putting the React app at the repo root. Rejected because it would mix Go and Node.js config files and make the monorepo harder to navigate.

### 4. SPA handler as catch-all after API routes
The `http.ServeMux` registers API routes first. A catch-all `GET /` handler serves the SPA. For any request that doesn't match a static file in the embedded FS, it serves `index.html` so React Router handles client-side routing.

### 5. Vite dev server with proxy for development
During development, Vite serves the frontend on :5173 with HMR. It proxies `/api` requests to the Go server on :8080. This gives instant frontend feedback without rebuilding. Air watches Go files and restarts the server on changes.

### 6. Placeholder pages for all routes from DESIGN.md
Even though Phase 1 doesn't implement business logic, the React app will have all route placeholders (`/`, `/s`, `/file`, `/login`, `/register`, `/history`) with stub components. This validates the routing setup early.

### 7. docker-compose for Postgres + MinIO only (not the app)
The Go app runs natively during development (via Air). docker-compose provides only the infrastructure services. This avoids the slow feedback loop of rebuilding a Docker image for the app on every change.

### 8. Config loads all env vars from DESIGN.md section 8, but only `SERVER_PORT` is used in Phase 1
The config struct is complete from the start so later phases just start using fields without changing the config package. Unused fields have sensible defaults and won't cause errors if their env vars aren't set.

## Risks / Trade-offs

- **Risk:** `embed.FS` requires the `web/frontend/dist/` directory to exist at compile time. If someone runs `go build` without building the frontend first, it fails. → **Mitigation:** The Makefile's `build` target runs `build-frontend` before `build-go`. A `.gitkeep` file in `web/frontend/dist/` ensures the directory exists for `go vet`/IDE indexing.

- **Risk:** Air + Vite running concurrently could confuse new contributors. → **Mitigation:** `make dev` handles both with clear output. README (added in Phase 7) documents the workflow.

- **Risk:** Tailwind CSS 4 is relatively new and some tooling may not support it fully. → **Mitigation:** Tailwind v4 has been stable since January 2025. The Vite plugin is the recommended integration path.

- **Trade-off:** Loading all config vars in Phase 1 when most aren't used yet means the config struct is "ahead" of the implementation. This is intentional — it's better than touching config.go in every subsequent phase.
