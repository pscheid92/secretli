### Requirement: CORS support for configured origins
The system SHALL handle CORS requests when `ALLOWED_ORIGINS` is configured.

#### Scenario: Same-origin mode (no CORS)
- **WHEN** `ALLOWED_ORIGINS` is empty (default)
- **THEN** the CORS middleware SHALL be a no-op and no `Access-Control-*` headers SHALL be added

#### Scenario: Allowed origin request
- **WHEN** a request includes an `Origin` header matching one of the `ALLOWED_ORIGINS`
- **THEN** the response SHALL include `Access-Control-Allow-Origin` set to the matching origin

#### Scenario: Disallowed origin request
- **WHEN** a request includes an `Origin` header NOT matching any `ALLOWED_ORIGINS`
- **THEN** the response SHALL NOT include any `Access-Control-Allow-Origin` header

### Requirement: CORS preflight handling
The system SHALL respond to CORS preflight `OPTIONS` requests.

#### Scenario: Preflight request for allowed origin
- **WHEN** an `OPTIONS` request is received with a valid `Origin` and `Access-Control-Request-Method` header
- **THEN** the response SHALL include `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS`, `Access-Control-Allow-Headers: Content-Type, X-Retrieval-Token, X-Deletion-Token`, `Access-Control-Allow-Credentials: true`, and `Access-Control-Max-Age: 86400`
- **AND** the response status SHALL be `204 No Content`

#### Scenario: Preflight request for disallowed origin
- **WHEN** an `OPTIONS` request is received with an `Origin` NOT in `ALLOWED_ORIGINS`
- **THEN** the response SHALL be `204 No Content` without CORS headers

### Requirement: CORS credentials support
The system SHALL allow credentials in cross-origin requests.

#### Scenario: Credentials allowed for valid origin
- **WHEN** a request comes from an allowed origin
- **THEN** the response SHALL include `Access-Control-Allow-Credentials: true`
