## MODIFIED Requirements

### Requirement: Loading spinner for file operations
The file upload and download UI SHALL show spinner components during async operations.

#### Scenario: Spinner during file upload
- **WHEN** a user submits a file for encryption and upload
- **THEN** a spinner SHALL be visible in the upload button

#### Scenario: Spinner during file download
- **WHEN** a user retrieves an encrypted file
- **THEN** a spinner SHALL be visible while downloading and decrypting

### Requirement: Toast feedback for file operations
File operations SHALL use toast notifications for success and error feedback.

#### Scenario: Upload success toast
- **WHEN** a file is successfully encrypted and uploaded
- **THEN** a success toast SHALL appear

#### Scenario: Upload error toast
- **WHEN** a file upload fails
- **THEN** an error toast SHALL appear with the error message
