## ADDED Requirements

### Requirement: Register API call
The API client SHALL export a `register(email, password, displayName)` function that sends `POST /api/v1/auth/register` with JSON body. It SHALL return `{ id, email, display_name, created_at }`.

#### Scenario: Successful registration
- **WHEN** `api.register(email, password, displayName)` is called
- **THEN** it sends a POST to `/api/v1/auth/register` and returns the user object

#### Scenario: Registration error
- **WHEN** the API returns 409 (duplicate email) or 400 (validation)
- **THEN** an `ApiError` is thrown with the status and error message

### Requirement: Login API call
The API client SHALL export a `login(email, password)` function that sends `POST /api/v1/auth/login` with JSON body. It SHALL return `{ id, email, display_name, created_at }`.

#### Scenario: Successful login
- **WHEN** `api.login(email, password)` is called with valid credentials
- **THEN** it sends a POST to `/api/v1/auth/login` and returns the user object

#### Scenario: Login error
- **WHEN** the API returns 401 (invalid credentials)
- **THEN** an `ApiError` is thrown with the status and error message

### Requirement: Logout API call
The API client SHALL export a `logout()` function that sends `POST /api/v1/auth/logout`. It SHALL return void.

#### Scenario: Successful logout
- **WHEN** `api.logout()` is called
- **THEN** it sends a POST to `/api/v1/auth/logout`

### Requirement: Get current user API call
The API client SHALL export a `getCurrentUser()` function that sends `GET /api/v1/auth/me`. It SHALL return `{ id, email, display_name, created_at }` or throw if unauthenticated.

#### Scenario: Authenticated user
- **WHEN** `api.getCurrentUser()` is called with a valid session
- **THEN** it returns the user object

#### Scenario: Unauthenticated
- **WHEN** `api.getCurrentUser()` is called without a session
- **THEN** an `ApiError` is thrown with status 401

### Requirement: Get user secrets API call
The API client SHALL export a `getUserSecrets(page?, perPage?)` function that sends `GET /api/v1/user/secrets` with query parameters. It SHALL return `{ secrets: [...], page, per_page, total }`.

#### Scenario: Fetch user secrets
- **WHEN** `api.getUserSecrets(1, 20)` is called
- **THEN** it sends a GET to `/api/v1/user/secrets?page=1&per_page=20` and returns the paginated result

#### Scenario: Unauthenticated
- **WHEN** `api.getUserSecrets()` is called without a session
- **THEN** an `ApiError` is thrown with status 401
