## MODIFIED Requirements

### Requirement: Middleware chain order
The middleware chain SHALL be applied using `chi.Router.Use()` in the following order: recovery → request ID → structured logging → security headers → CORS → session. Rate limiting SHALL be applied per route group rather than globally.

#### Scenario: Security headers applied before routing
- **WHEN** any request is processed
- **THEN** security headers SHALL be set before the request reaches route handlers

#### Scenario: CORS handled before session lookup
- **WHEN** a preflight OPTIONS request is received
- **THEN** CORS middleware SHALL respond before session middleware is evaluated

#### Scenario: Rate limiting applied per route group
- **WHEN** a request hits a rate-limited endpoint
- **THEN** the rate limit for that endpoint's group SHALL be checked
- **AND** other endpoint groups SHALL have independent rate limits

### Requirement: Recovery middleware
The server SHALL use chi's built-in `middleware.Recoverer` to recover from panics in request handlers without crashing. A panic SHALL result in an HTTP 500 response and a log entry.

#### Scenario: Handler panics
- **WHEN** a request handler panics
- **THEN** the server returns HTTP 500
- **AND** the panic is logged
- **AND** the server continues serving subsequent requests

### Requirement: Request ID middleware
The server SHALL use chi's built-in `middleware.RequestID` to assign a unique request ID to each incoming request and include it in the response header `X-Request-ID`. If the request already has an `X-Request-ID` header, the server SHALL use that value instead.

#### Scenario: New request ID generated
- **WHEN** a request arrives without an `X-Request-ID` header
- **THEN** a unique ID is generated and set as `X-Request-ID` in the response

#### Scenario: Existing request ID preserved
- **WHEN** a request arrives with `X-Request-ID: abc-123`
- **THEN** the response `X-Request-ID` header is `abc-123`

### Requirement: Structured logging middleware
The server SHALL log each request using a chi-compatible request logger that outputs structured fields via `log/slog`: method, path, status code, duration, and request ID.

#### Scenario: Request is logged
- **WHEN** any request completes
- **THEN** a structured log entry is emitted with method, path, status, duration_ms, and request_id fields

### Requirement: Auth route registration
The server SHALL register authentication endpoints using chi route groups: `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, and `GET /api/v1/auth/me`. Auth creation routes (`register`, `login`) SHALL be rate limited at 5 requests per minute.

#### Scenario: Auth routes registered with rate limiting
- **WHEN** the server starts
- **THEN** all four auth endpoints are available and routed to the appropriate handlers
- **AND** register and login endpoints are rate limited at 5 requests per minute

### Requirement: User route registration
The server SHALL register the user secrets history endpoint `GET /api/v1/user/secrets` within the API route group.

#### Scenario: User route registered
- **WHEN** the server starts
- **THEN** the user secrets history endpoint is available

### Requirement: URL parameter extraction
Handlers SHALL extract URL path parameters using `chi.URLParam(r, "paramName")` instead of `r.PathValue("paramName")`.

#### Scenario: Public ID extracted from URL
- **WHEN** a request is made to `/api/v1/secrets/{publicID}`
- **THEN** the handler extracts `publicID` using `chi.URLParam(r, "publicID")`
