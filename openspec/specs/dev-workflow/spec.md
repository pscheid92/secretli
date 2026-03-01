## ADDED Requirements

### Requirement: Makefile with dev target
The Makefile SHALL provide a `dev` target that concurrently starts the Go server (via Air) and the Vite dev server.

#### Scenario: make dev starts both servers
- **WHEN** `make dev` is run
- **THEN** the Go server starts on port 8080 with hot-reload via Air
- **AND** the Vite dev server starts on port 5173

### Requirement: Makefile with build target
The Makefile SHALL provide a `build` target that first builds the frontend (`npm run build` in `web/frontend/`), then builds the Go binary with the frontend embedded.

#### Scenario: make build produces single binary
- **WHEN** `make build` is run
- **THEN** `web/frontend/dist/` is populated by the frontend build
- **AND** a Go binary is produced at `bin/secretli`
- **AND** the binary contains the embedded frontend assets

### Requirement: Makefile with test target
The Makefile SHALL provide a `test` target that runs both Go tests and frontend tests.

#### Scenario: make test runs all tests
- **WHEN** `make test` is run
- **THEN** `go test ./...` runs for the Go backend
- **AND** `npm test` runs for the React frontend

### Requirement: Air configuration for Go hot-reload
An `.air.toml` file SHALL configure Air to watch `.go` and `.sql` files, exclude `web/frontend/`, `tmp/`, and `node_modules/`, and rebuild/restart the Go server on changes.

#### Scenario: Go file change triggers restart
- **WHEN** a `.go` file is modified while `make dev` is running
- **THEN** Air rebuilds and restarts the Go server automatically

### Requirement: Vite proxy for API requests
The Vite dev server SHALL proxy all requests matching `/api` to `http://localhost:8080` so the frontend can call the Go backend during development without CORS issues.

#### Scenario: API request proxied to Go backend
- **WHEN** the frontend makes a fetch request to `/api/v1/health/live` via the Vite dev server
- **THEN** the request is proxied to `http://localhost:8080/api/v1/health/live`
- **AND** the response is returned to the frontend

### Requirement: Docker Compose provides full local stack
The docker-compose.yml SHALL include an `app` service alongside postgres and minio for full local stack testing.

#### Scenario: App service builds from Dockerfile
- **WHEN** `docker compose up` is run
- **THEN** the app service SHALL build from the project Dockerfile and start the HTTP server on port 8080

#### Scenario: App connects to local services
- **WHEN** the app service starts
- **THEN** it SHALL connect to the postgres and minio services via Docker networking using appropriate environment variables

#### Scenario: App depends on infrastructure services
- **WHEN** docker compose starts services
- **THEN** the app service SHALL wait for postgres and minio to be ready before starting
