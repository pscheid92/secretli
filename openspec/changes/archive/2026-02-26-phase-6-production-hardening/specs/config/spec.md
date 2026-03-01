## MODIFIED Requirements

### Requirement: AllowedOrigins config is consumed by CORS middleware
The `ALLOWED_ORIGINS` environment variable SHALL be parsed as a comma-separated list of allowed origins and passed to the CORS middleware.

#### Scenario: Single allowed origin
- **WHEN** `ALLOWED_ORIGINS` is set to `https://example.com`
- **THEN** the CORS middleware SHALL allow requests from `https://example.com`

#### Scenario: Multiple allowed origins
- **WHEN** `ALLOWED_ORIGINS` is set to `https://example.com,https://app.example.com`
- **THEN** the CORS middleware SHALL allow requests from both origins

#### Scenario: Empty allowed origins
- **WHEN** `ALLOWED_ORIGINS` is empty or not set
- **THEN** the CORS middleware SHALL be a no-op (same-origin only mode)
