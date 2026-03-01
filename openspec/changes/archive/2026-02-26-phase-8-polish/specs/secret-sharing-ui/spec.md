## MODIFIED Requirements

### Requirement: Copy button on retrieved secret text
The RetrievePage SHALL provide a copy-to-clipboard button for the decrypted secret text.

#### Scenario: Copy decrypted text
- **WHEN** a text secret is successfully decrypted and displayed
- **THEN** a copy button SHALL be visible next to the secret text
- **AND** clicking it SHALL copy the text to clipboard and show a toast

### Requirement: Loading spinner during encryption and decryption
The UI SHALL show a spinner component during async operations instead of text-only indicators.

#### Scenario: Spinner during secret creation
- **WHEN** a user submits the secret form
- **THEN** a spinner SHALL be visible in the submit button while encrypting and sending

#### Scenario: Spinner during secret retrieval
- **WHEN** a user opens a share link and decryption is in progress
- **THEN** a spinner SHALL be visible on the page

### Requirement: Form validation on secret creation
The secret creation form SHALL validate inputs before submission.

#### Scenario: Empty text validation
- **WHEN** a user submits the secret form with empty text
- **THEN** a field-level error message SHALL appear below the textarea

#### Scenario: Password confirmation
- **WHEN** a user enables password protection
- **THEN** the password field SHALL be required and validated as non-empty before submission
