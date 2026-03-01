## MODIFIED Requirements

### Requirement: Server startup launches cleanup worker
The server startup sequence SHALL start the cleanup worker as a background goroutine alongside the HTTP server.

#### Scenario: Cleanup worker starts with server
- **WHEN** the server starts
- **THEN** the cleanup worker SHALL be started with the server's shutdown context, secret repo, session repo, and file store

#### Scenario: Cleanup worker stops on shutdown
- **WHEN** the server receives SIGINT/SIGTERM
- **THEN** the cleanup worker's context SHALL be cancelled, causing it to stop
- **AND** the server SHALL wait for the cleanup worker to finish before exiting

### Requirement: Middleware chain order
The middleware chain SHALL be applied in the following order: recovery → request ID → structured logging → security headers → CORS → rate limiting → session.

#### Scenario: Security headers applied before routing
- **WHEN** any request is processed
- **THEN** security headers SHALL be set before the request reaches route handlers

#### Scenario: CORS handled before rate limiting
- **WHEN** a preflight OPTIONS request is received
- **THEN** CORS middleware SHALL respond before rate limiting is evaluated
