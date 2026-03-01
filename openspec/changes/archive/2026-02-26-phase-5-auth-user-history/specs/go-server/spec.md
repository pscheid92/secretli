## MODIFIED Requirements

### Requirement: Structured logging middleware
The server SHALL log each request using `log/slog` with structured fields: method, path, status code, duration, request ID, and user ID (if authenticated).

#### Scenario: Request is logged
- **WHEN** any request completes
- **THEN** a structured log entry is emitted with method, path, status, duration_ms, and request_id fields

#### Scenario: Authenticated request is logged with user ID
- **WHEN** an authenticated request completes
- **THEN** the log entry includes the user_id field

## ADDED Requirements

### Requirement: Auth route registration
The server SHALL register authentication endpoints: `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, and `GET /api/v1/auth/me`.

#### Scenario: Auth routes registered
- **WHEN** the server starts
- **THEN** all four auth endpoints are available and routed to the appropriate handlers

### Requirement: User route registration
The server SHALL register the user secrets history endpoint: `GET /api/v1/user/secrets`.

#### Scenario: User route registered
- **WHEN** the server starts
- **THEN** the user secrets history endpoint is available

### Requirement: Session middleware in chain
The server SHALL include session middleware in the middleware chain, after logging and before route handling. The session middleware SHALL populate the request context with the authenticated user if a valid session exists.

#### Scenario: Middleware chain order
- **WHEN** a request is processed
- **THEN** the middleware executes in order: recovery, request ID, logging, session auth, then route handler
