## MODIFIED Requirements

### Requirement: Form validation on secret creation
The secret creation form SHALL use react-hook-form for state management and validation. All form fields SHALL be validated before submission with per-field inline error messages displayed below each respective input.

#### Scenario: Empty text validation
- **WHEN** a user submits the secret form with empty text
- **THEN** a field-level error message "Secret text is required" SHALL appear below the textarea

#### Scenario: Password required when protection enabled
- **WHEN** a user enables password protection and submits without entering a password
- **THEN** a field-level error message "Password is required" SHALL appear below the password input
- **AND** the form SHALL NOT submit

#### Scenario: Password not required when protection disabled
- **WHEN** a user submits the form without enabling password protection
- **THEN** no password validation error SHALL appear regardless of the password field value

### Requirement: SecretForm component
The SecretForm component SHALL provide a textarea for secret text, an ExpirationPicker (dropdown with options: 5m, 10m, 15m, 1h, 4h, 12h, 1d, 3d, 7d), a burn-after-read toggle, and an optional password field. Form state SHALL be managed by react-hook-form's `useForm` hook. The ExpirationPicker SHALL be integrated via `Controller`. The textarea and password fields SHALL use `register`.

#### Scenario: User fills out and submits the form
- **WHEN** the user enters text, selects expiration, and clicks "Share"
- **THEN** the form calls the `onSubmit` callback with `{ text, expiration, burnAfterRead, password }`

#### Scenario: Empty text submission is prevented
- **WHEN** the user tries to submit with an empty textarea
- **THEN** the form shows a validation error and does not submit

### Requirement: RetrievePage password form
The password entry form on the RetrievePage SHALL use react-hook-form for state management. The password field SHALL be validated as required with an inline error message.

#### Scenario: Empty password submission
- **WHEN** the user submits the password form without entering a password
- **THEN** a field-level error message "Password is required" SHALL appear below the password input

#### Scenario: Valid password submission
- **WHEN** the user enters a password and submits
- **THEN** the form calls the password submission handler with the entered password
