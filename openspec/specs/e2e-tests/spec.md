## Requirements

### Requirement: Playwright test infrastructure
The project SHALL have Playwright configured for end-to-end testing.

#### Scenario: Playwright config exists
- **WHEN** `npx playwright test` is run in the frontend directory
- **THEN** it SHALL find and execute test files

### Requirement: Text secret share-and-retrieve test
An e2e test SHALL verify the full text secret lifecycle.

#### Scenario: Create and retrieve text secret
- **WHEN** a user creates a text secret via the UI and opens the share link
- **THEN** the decrypted text SHALL match the original input

#### Scenario: Create and retrieve password-protected secret
- **WHEN** a user creates a password-protected secret and retrieves it with the correct password
- **THEN** the decrypted text SHALL match the original input

