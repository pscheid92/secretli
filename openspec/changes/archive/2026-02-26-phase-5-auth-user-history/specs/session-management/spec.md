## ADDED Requirements

### Requirement: Session creation
The server SHALL create sessions by generating 32 cryptographically random bytes, hex-encoding them to a 64-character string, and storing the session in the `sessions` table with the user ID and an expiration timestamp based on `SESSION_MAX_AGE` config (default 720 hours / 30 days).

#### Scenario: Session created on login
- **WHEN** a user successfully logs in
- **THEN** a session row is inserted with a random 64-character hex ID, the user's ID, and `expires_at` set to now + SESSION_MAX_AGE

#### Scenario: Session created on registration
- **WHEN** a user successfully registers
- **THEN** a session is created with the same properties as login

### Requirement: Session cookie format
The session cookie SHALL use the name `session_id` with attributes: `HttpOnly`, `Secure` (configurable via `COOKIE_SECURE`), `SameSite=Lax`, `Path=/`, `Max-Age` matching `SESSION_MAX_AGE` in seconds, and `Domain` set to `COOKIE_DOMAIN` (if configured).

#### Scenario: Cookie set on login
- **WHEN** a session is created
- **THEN** the `Set-Cookie` header includes `session_id=<hex>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=2592000`

#### Scenario: Cookie domain configured
- **WHEN** `COOKIE_DOMAIN` is set to `example.com`
- **THEN** the `Set-Cookie` header includes `Domain=example.com`

#### Scenario: Cookie secure disabled for development
- **WHEN** `COOKIE_SECURE` is `false`
- **THEN** the `Secure` flag is omitted from the cookie

### Requirement: Session middleware
The server SHALL include middleware that reads the `session_id` cookie, looks up the session in the database (joined with users), and attaches the user to the request context if the session is valid and not expired. The middleware SHALL NOT reject unauthenticated requests — it only populates context.

#### Scenario: Valid session populates user context
- **WHEN** a request includes a valid, non-expired `session_id` cookie
- **THEN** the authenticated user is available to handlers via request context

#### Scenario: No cookie passes through
- **WHEN** a request has no `session_id` cookie
- **THEN** the request proceeds with no user in context

#### Scenario: Expired session ignored
- **WHEN** a request includes a `session_id` for an expired session
- **THEN** the request proceeds with no user in context

#### Scenario: Invalid session ID ignored
- **WHEN** a request includes a `session_id` that does not exist in the database
- **THEN** the request proceeds with no user in context

### Requirement: Session deletion
The server SHALL delete a session from the database when the user logs out. The server SHALL clear the session cookie by setting `Max-Age=0`.

#### Scenario: Logout deletes session row
- **WHEN** a user logs out
- **THEN** the session row is deleted from the `sessions` table
- **AND** the `session_id` cookie is cleared

### Requirement: Sessions database table
The server SHALL use a `sessions` table with columns: `id` (TEXT PRIMARY KEY, hex-encoded random token), `user_id` (BIGINT NOT NULL, FK to users), `expires_at` (TIMESTAMPTZ NOT NULL), `created_at` (TIMESTAMPTZ NOT NULL DEFAULT NOW()). An index SHALL exist on `user_id` and on `expires_at`.

#### Scenario: Session table schema
- **WHEN** the sessions migration is applied
- **THEN** the table has `id`, `user_id`, `expires_at`, `created_at` columns with appropriate types and constraints
