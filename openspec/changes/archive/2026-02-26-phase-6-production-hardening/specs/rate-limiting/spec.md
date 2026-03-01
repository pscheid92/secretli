## ADDED Requirements

### Requirement: Per-IP token bucket rate limiting
The system SHALL enforce rate limits per client IP address using a token bucket algorithm.

#### Scenario: Request within rate limit
- **WHEN** a client IP has not exceeded its rate limit for the endpoint category
- **THEN** the request SHALL be processed normally

#### Scenario: Request exceeds rate limit
- **WHEN** a client IP exceeds its rate limit for the endpoint category
- **THEN** the server SHALL respond with `429 Too Many Requests` and a JSON body `{"error": "rate limit exceeded"}`

#### Scenario: Rate limit includes Retry-After header
- **WHEN** a request is rate limited
- **THEN** the response SHALL include a `Retry-After` header with the number of seconds to wait

### Requirement: Endpoint-specific rate limits
The system SHALL apply different rate limits based on endpoint category.

#### Scenario: Secret creation rate limit
- **WHEN** a request is made to `POST /api/v1/secrets` or `POST /api/v1/secrets/file`
- **THEN** the rate limit SHALL be 10 requests per minute per IP

#### Scenario: Secret retrieval rate limit
- **WHEN** a request is made to `POST /api/v1/secrets/{publicID}` or `POST /api/v1/secrets/{publicID}/file`
- **THEN** the rate limit SHALL be 30 requests per minute per IP

#### Scenario: Auth endpoint rate limit
- **WHEN** a request is made to any `/api/v1/auth/*` endpoint
- **THEN** the rate limit SHALL be 5 requests per minute per IP

#### Scenario: File upload rate limit
- **WHEN** a request is made to `POST /api/v1/secrets/file`
- **THEN** the rate limit SHALL be 5 requests per minute per IP (creation limit applies since it is lower than file-specific)

### Requirement: Rate limiter memory management
The system SHALL prevent unbounded memory growth from rate limiter state.

#### Scenario: Stale rate limiter cleanup
- **WHEN** a rate limiter entry for an IP has not been accessed for more than 10 minutes
- **THEN** the entry SHALL be removed from memory during the next cleanup sweep

### Requirement: Health and static endpoints are not rate limited
Health check and static file endpoints SHALL NOT be subject to rate limiting.

#### Scenario: Health check not rate limited
- **WHEN** a request is made to `/api/v1/health/live` or `/api/v1/health/ready`
- **THEN** the request SHALL NOT be rate limited
