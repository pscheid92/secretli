## MODIFIED Requirements

### Requirement: SecretResult component
The SecretResult component SHALL display the share URL, a copy-to-clipboard button, a QR code of the share URL, the expiration time, and whether burn-after-read is enabled. The QR code SHALL be rendered using the `qr-code` capability below the URL/copy row and above the metadata section.

#### Scenario: Display share link
- **WHEN** the SecretResult receives a share URL
- **THEN** it displays the full URL in a read-only input field

#### Scenario: Copy to clipboard
- **WHEN** the user clicks the copy button
- **THEN** the share URL is copied to the clipboard and the button shows a "Copied!" confirmation

#### Scenario: QR code displayed
- **WHEN** the SecretResult receives a share URL
- **THEN** a QR code encoding the full URL (including hash fragment) SHALL be visible below the copy row
