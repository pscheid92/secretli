## ADDED Requirements

### Requirement: User registration
The server SHALL accept `POST /api/v1/auth/register` with a JSON body containing `email`, `password`, and `display_name`. The server SHALL validate the email is not empty, the password is at least 8 characters, and the email is not already registered. The server SHALL hash the password with bcrypt (cost 12) and store the user. On success, the server SHALL create a session and return `201 Created` with a `Set-Cookie` header and the user object (without password hash).

#### Scenario: Successful registration
- **WHEN** a POST request is made to `/api/v1/auth/register` with valid `email`, `password`, and `display_name`
- **THEN** the user is created in the database with a bcrypt-hashed password
- **AND** a session is created
- **AND** the response status is 201 with `Set-Cookie: session_id=<hex>; HttpOnly; Secure; SameSite=Lax; Path=/`
- **AND** the body contains `{ id, email, display_name, created_at }`

#### Scenario: Duplicate email rejected
- **WHEN** a POST request uses an email that already exists
- **THEN** the response status is 409 Conflict with `{ "error": "email already registered" }`

#### Scenario: Password too short
- **WHEN** the password is fewer than 8 characters
- **THEN** the response status is 400 Bad Request

#### Scenario: Missing required fields
- **WHEN** the email or password field is missing
- **THEN** the response status is 400 Bad Request

### Requirement: User login
The server SHALL accept `POST /api/v1/auth/login` with a JSON body containing `email` and `password`. The server SHALL verify the password against the stored bcrypt hash. On success, the server SHALL create a session and return `200 OK` with a `Set-Cookie` header and the user object.

#### Scenario: Successful login
- **WHEN** a POST request is made with valid email and password
- **THEN** a session is created
- **AND** the response status is 200 with `Set-Cookie` header
- **AND** the body contains `{ id, email, display_name, created_at }`

#### Scenario: Wrong password
- **WHEN** the password does not match the stored hash
- **THEN** the response status is 401 Unauthorized with `{ "error": "invalid credentials" }`

#### Scenario: Email not found
- **WHEN** the email is not registered
- **THEN** the response status is 401 Unauthorized with `{ "error": "invalid credentials" }`

### Requirement: User logout
The server SHALL accept `POST /api/v1/auth/logout`. If a valid session cookie is present, the server SHALL delete the session from the database and clear the cookie. The response SHALL always be `204 No Content`.

#### Scenario: Successful logout
- **WHEN** a POST request is made with a valid session cookie
- **THEN** the session is deleted from the database
- **AND** the `session_id` cookie is cleared (Max-Age=0)
- **AND** the response status is 204

#### Scenario: Logout without session
- **WHEN** a POST request is made without a session cookie
- **THEN** the response status is 204 (no error)

### Requirement: Get current user
The server SHALL accept `GET /api/v1/auth/me`. If a valid session exists, the server SHALL return the user object. If no valid session exists, the server SHALL return 401.

#### Scenario: Authenticated user
- **WHEN** a GET request is made with a valid session cookie
- **THEN** the response status is 200 with `{ id, email, display_name, created_at }`

#### Scenario: No session
- **WHEN** a GET request is made without a session cookie
- **THEN** the response status is 401 Unauthorized

#### Scenario: Expired session
- **WHEN** a GET request is made with an expired session cookie
- **THEN** the response status is 401 Unauthorized
