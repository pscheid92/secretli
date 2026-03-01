## MODIFIED Requirements

### Requirement: Health readiness endpoint
The server SHALL expose `GET /api/v1/health/ready` that checks database connectivity by pinging the PostgreSQL connection pool. If the database is reachable, it returns HTTP 200 with `{"status":"ok"}`. If the database is unreachable, it returns HTTP 503 with `{"status":"unavailable"}`.

#### Scenario: Database is healthy
- **WHEN** a GET request is made to `/api/v1/health/ready`
- **AND** the database is reachable
- **THEN** the response status is 200 and the body is `{"status":"ok"}`

#### Scenario: Database is unhealthy
- **WHEN** a GET request is made to `/api/v1/health/ready`
- **AND** the database is unreachable
- **THEN** the response status is 503 and the body is `{"status":"unavailable"}`
