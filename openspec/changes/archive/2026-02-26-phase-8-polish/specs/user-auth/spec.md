## MODIFIED Requirements

### Requirement: Field-level form validation on auth pages
Login and register forms SHALL validate fields with inline error messages.

#### Scenario: Invalid email format
- **WHEN** a user enters an invalid email and blurs the field
- **THEN** an error message SHALL appear below the email input

#### Scenario: Password too short
- **WHEN** a user enters a password shorter than 8 characters and blurs the field
- **THEN** an error message SHALL appear below the password input

#### Scenario: Required field empty
- **WHEN** a user blurs a required field that is empty
- **THEN** an error message SHALL appear indicating the field is required

### Requirement: Loading spinner on auth buttons
Auth form submit buttons SHALL show a spinner during submission.

#### Scenario: Login button spinner
- **WHEN** a user submits the login form
- **THEN** the button SHALL show a spinner and be disabled until the response
