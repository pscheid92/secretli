## Requirements

### Requirement: Global toast notification system
The application SHALL provide a global toast notification system using sonner for success, error, and info feedback.

#### Scenario: Toaster mounted at root
- **WHEN** the application renders
- **THEN** a `<Toaster />` component SHALL be present in the root layout

#### Scenario: Toast respects theme
- **WHEN** a toast is displayed in dark mode
- **THEN** it SHALL use dark-mode-appropriate styling

### Requirement: Toast on secret creation
The application SHALL show a success toast when a secret is created.

#### Scenario: Text secret created
- **WHEN** a user successfully creates a text secret
- **THEN** a success toast SHALL appear with a message like "Secret created"

#### Scenario: File secret uploaded
- **WHEN** a user successfully uploads an encrypted file
- **THEN** a success toast SHALL appear with a message like "File uploaded"

### Requirement: Toast on copy-to-clipboard
The application SHALL show a toast when content is copied to clipboard.

#### Scenario: Share link copied
- **WHEN** a user copies the share link from SecretResult
- **THEN** a success toast SHALL appear confirming the copy

#### Scenario: Retrieved secret text copied
- **WHEN** a user copies the decrypted secret text
- **THEN** a success toast SHALL appear confirming the copy

### Requirement: Toast on error
The application SHALL show an error toast for API and network errors.

#### Scenario: API error
- **WHEN** an API call fails with an error response
- **THEN** an error toast SHALL appear with a user-friendly message

#### Scenario: Network error
- **WHEN** a network request fails (offline, timeout)
- **THEN** an error toast SHALL appear indicating connectivity issues

