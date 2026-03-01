## ADDED Requirements

### Requirement: Form validation on file upload
The file upload form SHALL use react-hook-form for state management and validation of non-file fields (expiration, password, burn-after-read). File selection SHALL be bridged to react-hook-form via `setValue`. All fields SHALL display per-field inline error messages.

#### Scenario: No file selected
- **WHEN** the user submits the file upload form without selecting a file
- **THEN** a field-level error message "Please select a file" SHALL appear below the file upload area

#### Scenario: Password required when protection enabled
- **WHEN** the user enables password protection on the file form and submits without entering a password
- **THEN** a field-level error message "Password is required" SHALL appear below the password input

#### Scenario: Valid file form submission
- **WHEN** the user selects a file, configures options, and submits
- **THEN** react-hook-form validates all fields and calls the submit handler with form data

## MODIFIED Requirements

### Requirement: FilePage orchestration
The FilePage SHALL render a FileUpload component, an ExpirationPicker, a burn-after-read toggle, an optional password field, and a submit button. Form state for non-file fields SHALL be managed by react-hook-form's `useForm` hook. The ExpirationPicker SHALL be integrated via `Controller`. File selection SHALL be synced to form state via `setValue`. On submit, it SHALL generate a KeySet, encrypt the file and filename client-side, upload via `POST /api/v1/secrets/file`, and display a share link via SecretResult.

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
