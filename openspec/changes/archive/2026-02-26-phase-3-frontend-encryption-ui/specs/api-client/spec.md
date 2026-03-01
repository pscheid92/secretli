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
