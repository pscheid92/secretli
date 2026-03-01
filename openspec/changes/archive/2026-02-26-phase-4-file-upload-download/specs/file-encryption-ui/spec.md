## ADDED Requirements

### Requirement: FileUpload component
The FileUpload component SHALL provide a drag-and-drop zone and a file input button for selecting a single file. It SHALL display the selected filename and file size. It SHALL reject files exceeding 100MB with an error message.

#### Scenario: Select file via click
- **WHEN** the user clicks the upload area and selects a file
- **THEN** the component displays the filename and size and calls the `onSelect` callback with the File object

#### Scenario: Select file via drag-and-drop
- **WHEN** the user drags and drops a file onto the upload area
- **THEN** the component displays the filename and size and calls the `onSelect` callback

#### Scenario: File exceeds 100MB
- **WHEN** the user selects a file larger than 100MB
- **THEN** the component displays an error message and does not call `onSelect`

#### Scenario: Clear selected file
- **WHEN** the user clicks a remove/clear button on the selected file
- **THEN** the selection is cleared and `onSelect` is called with null

### Requirement: FilePage orchestration
The FilePage SHALL render a FileUpload component, an ExpirationPicker, a burn-after-read toggle, an optional password field, and a submit button. On submit, it SHALL generate a KeySet, encrypt the file and filename client-side, upload via `POST /api/v1/secrets/file`, and display a share link via SecretResult.

#### Scenario: Successful file upload
- **WHEN** the user selects a file, configures options, and clicks "Share File"
- **THEN** the file and filename are encrypted client-side, uploaded to the API, and a share link is displayed

#### Scenario: File upload with password
- **WHEN** the user uploads a file with a password set
- **THEN** keys are derived with the password, `password_protected: true` is sent to the API, and the share link works the same way

#### Scenario: Loading state during upload
- **WHEN** encryption and upload are in progress
- **THEN** the submit button shows a loading state and is disabled

#### Scenario: API error during upload
- **WHEN** the API returns an error
- **THEN** the page displays the error message

### Requirement: RetrievePage file download
The RetrievePage SHALL detect `secret_type: "file"` from the text retrieval response and fetch the encrypted file from `POST /api/v1/secrets/{publicID}/file`. It SHALL decrypt the file and filename client-side and trigger a browser download with the original filename.

#### Scenario: Retrieve and download a file secret
- **WHEN** the user navigates to a share link for a file secret
- **THEN** the page detects file type, downloads the encrypted blob, decrypts it and the filename, and triggers a browser file save

#### Scenario: File secret with password
- **WHEN** a file secret is password-protected
- **THEN** the page prompts for a password, re-derives keys, and then downloads and decrypts the file

#### Scenario: File secret with deletion token
- **WHEN** the URL contains a deletion token for a file secret
- **THEN** a "Delete" button is shown that deletes both the database record and S3 object
