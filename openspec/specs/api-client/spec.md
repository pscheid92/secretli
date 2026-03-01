## ADDED Requirements

### Requirement: Typed API client
The `lib/api.ts` module SHALL export typed functions for each API endpoint. All requests SHALL use `credentials: "same-origin"` for session cookies. All requests SHALL set `Content-Type: application/json` for JSON bodies.

#### Scenario: Create secret API call
- **WHEN** `api.createSecret({ public_id, retrieval_token, deletion_token, nonce, encrypted_data, expiration, burn_after_read, password_protected })` is called
- **THEN** it sends `POST /api/v1/secrets` with the JSON body and returns `{ expires_at: string }`

#### Scenario: Retrieve secret API call
- **WHEN** `api.retrieveSecret(publicID, retrievalToken)` is called
- **THEN** it sends `POST /api/v1/secrets/{publicID}` with `X-Retrieval-Token` header and returns `{ nonce, encrypted_data, secret_type, burn_after_read, password_protected }`

#### Scenario: Delete secret API call
- **WHEN** `api.deleteSecret(publicID, retrievalToken, deletionToken)` is called
- **THEN** it sends `DELETE /api/v1/secrets/{publicID}` with `X-Retrieval-Token` and `X-Deletion-Token` headers

### Requirement: ApiError class
The API client SHALL throw an `ApiError` with `status` (number) and `message` (string) properties for any non-2xx response.

#### Scenario: 404 response
- **WHEN** the API returns a 404 status
- **THEN** an `ApiError` is thrown with `status: 404` and the server's error message

#### Scenario: 400 response with error body
- **WHEN** the API returns a 400 status with `{ "error": "missing field" }`
- **THEN** an `ApiError` is thrown with `status: 400` and `message: "missing field"`

#### Scenario: Network error
- **WHEN** the fetch request fails due to a network error
- **THEN** an `ApiError` is thrown with a descriptive message

### Requirement: Upload file API call
The API client SHALL export an `uploadFile(metadata, encryptedBlob)` function that sends `POST /api/v1/secrets/file` as `multipart/form-data` with a `metadata` JSON part and a `file` binary part. It SHALL return `{ expires_at: string }`.

#### Scenario: Successful file upload
- **WHEN** `api.uploadFile(metadata, blob)` is called
- **THEN** it sends a multipart POST to `/api/v1/secrets/file` with the metadata JSON and encrypted blob
- **AND** returns `{ expires_at: string }`

#### Scenario: Upload error
- **WHEN** the API returns an error (400, 409, etc.)
- **THEN** an `ApiError` is thrown with the status and error message

### Requirement: Download file API call
The API client SHALL export a `downloadFile(publicID, retrievalToken)` function that sends `POST /api/v1/secrets/{publicID}/file` with an `X-Retrieval-Token` header. It SHALL return `{ blob: Blob, encryptedFilename: string, burnAfterRead: boolean, passwordProtected: boolean }`.

#### Scenario: Successful file download
- **WHEN** `api.downloadFile(publicID, retrievalToken)` is called
- **THEN** it sends a POST to `/api/v1/secrets/{publicID}/file` with the retrieval token header
- **AND** returns the response body as a Blob and the `X-Encrypted-Filename` header value

#### Scenario: Download error
- **WHEN** the API returns 404 or 403
- **THEN** an `ApiError` is thrown with the appropriate status and message

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
