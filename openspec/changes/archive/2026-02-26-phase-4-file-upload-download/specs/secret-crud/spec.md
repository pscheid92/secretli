## MODIFIED Requirements

### Requirement: Delete a secret
The server SHALL accept `DELETE /api/v1/secrets/{publicID}` with both `X-Retrieval-Token` and `X-Deletion-Token` headers. Both tokens MUST match their stored hashes for the deletion to proceed. If the secret has `secret_type = "file"`, the server SHALL also delete the corresponding S3 object at key `secrets/{publicID}`.

#### Scenario: Successful deletion
- **WHEN** a DELETE request provides valid retrieval and deletion tokens
- **THEN** the secret is deleted from the database
- **AND** the response status is 204 No Content

#### Scenario: Successful deletion of file secret
- **WHEN** a DELETE request targets a file-type secret with valid tokens
- **THEN** the secret is deleted from the database
- **AND** the S3 object at `secrets/{publicID}` is deleted

#### Scenario: Invalid deletion token
- **WHEN** the `X-Deletion-Token` does not match
- **THEN** the response status is 403 Forbidden

#### Scenario: Missing deletion token header
- **WHEN** the `X-Deletion-Token` header is absent
- **THEN** the response status is 400 Bad Request

### Requirement: Burn-after-read
When a secret has `burn_after_read` set to true, the server SHALL delete the secret from the database atomically upon successful retrieval. Only the first retrieval request SHALL receive the data. If the secret has `secret_type = "file"`, the S3 object SHALL also be deleted after the file has been fully streamed to the client.

#### Scenario: Burn-after-read secret deleted after retrieval
- **WHEN** a burn-after-read secret is retrieved successfully
- **THEN** the secret data is returned
- **AND** the secret is deleted from the database

#### Scenario: Burn-after-read file secret
- **WHEN** a burn-after-read file secret is downloaded
- **THEN** the encrypted file is streamed to the client
- **AND** the database record and S3 object are deleted

#### Scenario: Second retrieval of burn-after-read secret fails
- **WHEN** a burn-after-read secret has already been retrieved
- **THEN** the response status is 404 Not Found

## ADDED Requirements

### Requirement: Secret repository includes file columns
The secret repository `Create` method SHALL persist `storage_key`, `encrypted_filename`, and `encrypted_size` columns. The `GetByPublicID` and `GetAndDeleteByPublicID` methods SHALL return these fields on the `Secret` model. The `DeleteExpired` method SHALL return the storage keys of deleted file-type secrets so the caller can clean up S3 objects.

#### Scenario: Create file secret in database
- **WHEN** a secret with `secret_type: "file"` is created
- **THEN** the `storage_key`, `encrypted_filename`, and `encrypted_size` columns are populated

#### Scenario: Create text secret preserves null file columns
- **WHEN** a secret with `secret_type: "text"` is created
- **THEN** the `storage_key`, `encrypted_filename`, and `encrypted_size` columns are NULL

#### Scenario: Retrieve file secret includes file metadata
- **WHEN** `GetByPublicID` is called for a file secret
- **THEN** the returned `Secret` model includes `StorageKey`, `EncryptedFilename`, and `EncryptedSize`

#### Scenario: DeleteExpired returns storage keys
- **WHEN** `DeleteExpired` deletes file-type secrets
- **THEN** it returns the storage keys of the deleted secrets
