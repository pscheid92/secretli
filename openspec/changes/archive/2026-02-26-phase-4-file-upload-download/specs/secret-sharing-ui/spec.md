## MODIFIED Requirements

### Requirement: RetrievePage decryption flow
The RetrievePage SHALL read the URL hash fragment, extract the share secret (and optional deletion token), derive keys, call `POST /api/v1/secrets/{publicID}` with the retrieval token, and handle the response based on `secret_type`. For text secrets, it SHALL decrypt and display the plaintext. For file secrets, it SHALL fetch the encrypted file from `POST /api/v1/secrets/{publicID}/file`, decrypt the file and filename client-side, and trigger a browser file download with the original filename.

#### Scenario: Retrieve and decrypt a text secret
- **WHEN** the user navigates to `/s#<shareSecret>` and the secret type is "text"
- **THEN** the page derives keys, calls the retrieval API, decrypts the response, and displays the plaintext

#### Scenario: Retrieve and download a file secret
- **WHEN** the user navigates to `/s#<shareSecret>` and the secret type is "file"
- **THEN** the page derives keys, calls the text retrieval API to get metadata, then calls the file download API, decrypts the file and filename, and triggers a browser file save

#### Scenario: Retrieve with deletion token in URL
- **WHEN** the URL is `/s#<shareSecret>!<deletionToken>`
- **THEN** the page extracts both the share secret and deletion token, and shows a "Delete" button

#### Scenario: Password-protected secret
- **WHEN** the API response indicates `password_protected: true`
- **THEN** the page prompts the user for a password, re-derives keys with the password, and attempts decryption

#### Scenario: Password-protected file secret
- **WHEN** a file secret is password-protected
- **THEN** the page prompts for a password, re-derives keys, then downloads and decrypts the file

#### Scenario: Wrong password
- **WHEN** the user enters an incorrect password for a password-protected secret
- **THEN** the page shows an error message and allows retry

#### Scenario: Secret not found
- **WHEN** the API returns 404
- **THEN** the page shows "This secret has expired or does not exist"

#### Scenario: Secret already burned
- **WHEN** the API returns 404 for a burn-after-read secret
- **THEN** the page shows "This secret has already been viewed and was destroyed"
