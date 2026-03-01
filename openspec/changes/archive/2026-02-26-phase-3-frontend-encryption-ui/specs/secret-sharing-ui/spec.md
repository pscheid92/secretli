## ADDED Requirements

### Requirement: SecretForm component
The SecretForm component SHALL provide a textarea for secret text, an ExpirationPicker (dropdown with options: 5m, 10m, 15m, 1h, 4h, 12h, 1d, 3d, 7d), a burn-after-read toggle, and an optional password field.

#### Scenario: User fills out and submits the form
- **WHEN** the user enters text, selects expiration, and clicks "Share"
- **THEN** the form calls the `onSubmit` callback with `{ text, expiration, burnAfterRead, password }`

#### Scenario: Empty text submission is prevented
- **WHEN** the user tries to submit with an empty textarea
- **THEN** the form shows a validation error and does not submit

### Requirement: ExpirationPicker component
The ExpirationPicker SHALL render a select dropdown with the allowed expiration values: 5m, 10m, 15m, 1h, 4h, 12h, 1d, 3d, 7d. The default selection SHALL be 1d.

#### Scenario: Default expiration
- **WHEN** the ExpirationPicker renders
- **THEN** "1 day" is selected by default

#### Scenario: User selects expiration
- **WHEN** the user selects "7 days" from the dropdown
- **THEN** the `onChange` callback is called with `"7d"`

### Requirement: SecretResult component
The SecretResult component SHALL display the share URL and a copy-to-clipboard button. It SHALL also show the expiration time and whether burn-after-read is enabled.

#### Scenario: Display share link
- **WHEN** the SecretResult receives a share URL
- **THEN** it displays the full URL in a read-only input field

#### Scenario: Copy to clipboard
- **WHEN** the user clicks the copy button
- **THEN** the share URL is copied to the clipboard and the button shows a "Copied!" confirmation

### Requirement: SharePage orchestration
The SharePage SHALL orchestrate the full secret creation flow: render SecretForm, on submit generate a KeySet (with optional password), encrypt the text, send to `POST /api/v1/secrets`, and display the result via SecretResult.

#### Scenario: Successful secret creation
- **WHEN** the user submits a valid secret
- **THEN** the page encrypts the text client-side, sends the encrypted data to the API, and displays a share link in the format `/s#<shareSecret>!<deletionToken>`

#### Scenario: Secret creation with password
- **WHEN** the user submits a secret with a password
- **THEN** the `password_protected: true` flag is sent to the API, and the share link is the same format (password is not in the URL)

#### Scenario: API error during creation
- **WHEN** the API returns an error
- **THEN** the page displays the error message to the user

#### Scenario: Loading state during encryption
- **WHEN** the user clicks "Share" and encryption/API call is in progress
- **THEN** the submit button shows a loading indicator and is disabled

### Requirement: RetrievePage decryption flow
The RetrievePage SHALL read the URL hash fragment, extract the share secret (and optional deletion token), derive keys, call `POST /api/v1/secrets/{publicID}` with the retrieval token, decrypt the response, and display the plaintext.

#### Scenario: Retrieve and decrypt a text secret
- **WHEN** the user navigates to `/s#<shareSecret>`
- **THEN** the page derives keys from the share secret, calls the retrieval API, decrypts the response, and displays the plaintext

#### Scenario: Retrieve with deletion token in URL
- **WHEN** the URL is `/s#<shareSecret>!<deletionToken>`
- **THEN** the page extracts both the share secret and deletion token, and shows a "Delete" button

#### Scenario: Password-protected secret
- **WHEN** the API response indicates `password_protected: true`
- **THEN** the page prompts the user for a password, re-derives keys with the password, and attempts decryption

#### Scenario: Wrong password
- **WHEN** the user enters an incorrect password for a password-protected secret
- **THEN** the page shows an error message and allows retry

#### Scenario: Secret not found
- **WHEN** the API returns 404
- **THEN** the page shows "This secret has expired or does not exist"

#### Scenario: Secret already burned
- **WHEN** the API returns 404 for a burn-after-read secret
- **THEN** the page shows "This secret has already been viewed and was destroyed"

### Requirement: Delete secret from RetrievePage
When the URL contains a deletion token, the RetrievePage SHALL show a "Delete" button that calls `DELETE /api/v1/secrets/{publicID}` with both retrieval and deletion tokens.

#### Scenario: Delete a secret
- **WHEN** the user clicks "Delete" on the RetrievePage
- **THEN** the page sends a DELETE request with both tokens and shows a confirmation message

### Requirement: Share URL format
The share URL SHALL use the format `{origin}/s#{shareSecret}` for basic sharing and `{origin}/s#{shareSecret}!{deletionToken}` when including the deletion token. Both `shareSecret` and `deletionToken` are URL-safe base64 encoded.

#### Scenario: URL without deletion token
- **WHEN** a share link is generated without deletion token
- **THEN** the URL format is `https://example.com/s#<base64url>`

#### Scenario: URL with deletion token
- **WHEN** a share link is generated with deletion token
- **THEN** the URL format is `https://example.com/s#<base64url>!<base64url>`
