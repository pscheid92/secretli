### Requirement: Multi-stage container build
The Dockerfile SHALL use three stages to produce a minimal container image.

#### Scenario: Stage 1 builds frontend
- **WHEN** the Docker build runs stage 1 (node:22-alpine)
- **THEN** it SHALL run `npm ci` and `npm run build` in `web/frontend/` to produce the `dist/` directory

#### Scenario: Stage 2 builds Go binary
- **WHEN** the Docker build runs stage 2 (golang:1.23-alpine)
- **THEN** it SHALL copy the frontend `dist/` into `web/frontend/dist/`, download Go modules, and build the binary with `CGO_ENABLED=0`

#### Scenario: Stage 3 produces final image
- **WHEN** the Docker build runs stage 3 (gcr.io/distroless/static-debian12)
- **THEN** the final image SHALL contain only the Go binary and no shell, package manager, or build tools

### Requirement: Minimal image size
The final container image SHALL be as small as possible.

#### Scenario: Image size under 30MB
- **WHEN** the Docker image is built
- **THEN** the compressed image size SHALL be under 30MB

### Requirement: Container runs as non-root
The container SHALL run the application as a non-root user.

#### Scenario: Non-root execution
- **WHEN** the container starts
- **THEN** the process SHALL run as the `nonroot` user (UID 65532) provided by distroless

### Requirement: Container exposes port 8080
The container SHALL expose port 8080 for the HTTP server.

#### Scenario: Port exposure
- **WHEN** the Dockerfile is built
- **THEN** it SHALL declare `EXPOSE 8080`

### Requirement: Binary accepts CLI arguments
The container entrypoint SHALL allow passing subcommands (e.g., `migrate`).

#### Scenario: Default entrypoint runs server
- **WHEN** the container starts with no arguments
- **THEN** it SHALL run the secretli binary which starts the HTTP server

#### Scenario: Migrate subcommand
- **WHEN** the container starts with `migrate` argument
- **THEN** it SHALL run database migrations and exit
