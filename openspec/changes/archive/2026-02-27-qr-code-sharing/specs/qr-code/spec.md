## ADDED Requirements

### Requirement: QR code rendered from share URL
The `SecretResult` component SHALL render a QR code encoding the full share URL, including the `#` fragment, using the `qrcode` npm library. The QR code SHALL be generated client-side with error correction level `M` and displayed as an `<img>` element with a `data:image/png;base64,...` src.

#### Scenario: QR code appears after secret creation
- **WHEN** a secret is created and `SecretResult` renders with a valid `url` prop
- **THEN** a QR code image encoding that URL SHALL be visible below the copy row

#### Scenario: QR code encodes complete URL including hash fragment
- **WHEN** the share URL is `https://example.com/s#abc123!def456`
- **THEN** the QR code data SHALL encode the full string `https://example.com/s#abc123!def456`

#### Scenario: QR code works for file secrets
- **WHEN** a file secret is created and `SecretResult` renders
- **THEN** a QR code SHALL be rendered in the same position as for text secrets

### Requirement: QR code dark mode adaptation
The QR code image SHALL always render with a white background and black modules, regardless of the page theme. In dark mode the component SHALL wrap the image in a white-padded container so it stands out against the dark card background.

#### Scenario: QR code visible in dark mode
- **WHEN** dark mode is active and `SecretResult` renders
- **THEN** the QR code SHALL have a white background and remain high-contrast and scannable

#### Scenario: QR code visible in light mode
- **WHEN** light mode is active and `SecretResult` renders
- **THEN** the QR code SHALL have a white background and remain high-contrast and scannable

### Requirement: QR code display size
The QR code image SHALL be displayed at 160×160 px via CSS, with the underlying generated image being at the library default resolution (256×256 px), ensuring crispness on high-DPI screens.

#### Scenario: QR code rendered at correct display size
- **WHEN** `SecretResult` renders a QR code
- **THEN** the `<img>` element SHALL have CSS width and height of 160 px
