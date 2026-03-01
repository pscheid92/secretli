## ADDED Requirements

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
