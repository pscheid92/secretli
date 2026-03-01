## ADDED Requirements

### Requirement: Security headers on all responses
The system SHALL add security headers to every HTTP response.

#### Scenario: X-Content-Type-Options header
- **WHEN** any HTTP response is sent
- **THEN** it SHALL include the header `X-Content-Type-Options: nosniff`

#### Scenario: X-Frame-Options header
- **WHEN** any HTTP response is sent
- **THEN** it SHALL include the header `X-Frame-Options: DENY`

#### Scenario: Content-Security-Policy header
- **WHEN** any HTTP response is sent
- **THEN** it SHALL include the header `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'`

#### Scenario: Referrer-Policy header
- **WHEN** any HTTP response is sent
- **THEN** it SHALL include the header `Referrer-Policy: no-referrer`

### Requirement: Referrer-Policy prevents secret leakage
The `Referrer-Policy: no-referrer` header is critical to prevent URL fragments (containing share secrets) from leaking via the Referer header.

#### Scenario: No referrer sent on navigation
- **WHEN** a user clicks an external link from the application
- **THEN** the browser SHALL NOT send a Referer header containing the application URL
