## ADDED Requirements

### Requirement: HTTP server listens on configured port
The Go HTTP server SHALL listen on the port specified by the `SERVER_PORT` environment variable, defaulting to `8080`.

#### Scenario: Server starts on default port
- **WHEN** `SERVER_PORT` is not set
- **THEN** the server listens on port 8080

#### Scenario: Server starts on custom port
- **WHEN** `SERVER_PORT` is set to `9090`
- **THEN** the server listens on port 9090

### Requirement: Health liveness endpoint
The server SHALL expose `GET /api/v1/health/live` that returns HTTP 200 with body `{"status":"ok"}`.

#### Scenario: Liveness check
- **WHEN** a GET request is made to `/api/v1/health/live`
- **THEN** the response status is 200 and the body is `{"status":"ok"}`

### Requirement: Health readiness endpoint
The server SHALL expose `GET /api/v1/health/ready` that returns HTTP 200 with body `{"status":"ok"}`. In Phase 1, this is identical to liveness. In later phases, it will check DB and S3 connectivity.

#### Scenario: Readiness check
- **WHEN** a GET request is made to `/api/v1/health/ready`
- **THEN** the response status is 200 and the body is `{"status":"ok"}`

### Requirement: SPA handler serves embedded frontend
The server SHALL serve the embedded React build output for all non-API paths. If the requested path matches a static file in the embedded filesystem, that file SHALL be served. Otherwise, `index.html` SHALL be served to support client-side routing.

#### Scenario: Serve index.html for root path
- **WHEN** a GET request is made to `/`
- **THEN** the response contains the React app's `index.html`

#### Scenario: Serve static asset
- **WHEN** a GET request is made to `/assets/index-abc123.js`
- **AND** that file exists in the embedded filesystem
- **THEN** the file is served with the correct content type

#### Scenario: SPA fallback for client-side routes
- **WHEN** a GET request is made to `/login`
- **AND** no file named `login` exists in the embedded filesystem
- **THEN** `index.html` is served so React Router handles the route

### Requirement: Recovery middleware
The server SHALL recover from panics in request handlers without crashing. A panic SHALL result in an HTTP 500 response and a structured log entry.

#### Scenario: Handler panics
- **WHEN** a request handler panics
- **THEN** the server returns HTTP 500
- **AND** the panic is logged with `slog.Error`
- **AND** the server continues serving subsequent requests

### Requirement: Request ID middleware
The server SHALL assign a unique request ID to each incoming request and include it in the response header `X-Request-ID`. If the request already has an `X-Request-ID` header, the server SHALL use that value instead.

#### Scenario: New request ID generated
- **WHEN** a request arrives without an `X-Request-ID` header
- **THEN** a UUID is generated and set as `X-Request-ID` in the response

#### Scenario: Existing request ID preserved
- **WHEN** a request arrives with `X-Request-ID: abc-123`
- **THEN** the response `X-Request-ID` header is `abc-123`

### Requirement: Structured logging middleware
The server SHALL log each request using `log/slog` with structured fields: method, path, status code, duration, and request ID.

#### Scenario: Request is logged
- **WHEN** any request completes
- **THEN** a structured log entry is emitted with method, path, status, duration_ms, and request_id fields

### Requirement: Graceful shutdown
The server SHALL shut down gracefully when it receives SIGINT or SIGTERM. It SHALL stop accepting new connections, wait for in-flight requests to complete (up to 30 seconds), then exit.

#### Scenario: Graceful shutdown on SIGTERM
- **WHEN** the server receives SIGTERM
- **THEN** it stops accepting new connections
- **AND** waits up to 30 seconds for in-flight requests
- **AND** exits with code 0
