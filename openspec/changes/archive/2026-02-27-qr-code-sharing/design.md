## Context

`SecretResult` currently shows a read-only URL input and a copy button. Users who want to share a secret with a phone must manually copy the URL and send it via another channel. The URL includes a hash fragment (`#<shareSecret>`) which encodes the encryption key — it must be preserved exactly for the recipient to decrypt the secret.

## Goals / Non-Goals

**Goals:**
- Render a QR code of the full share URL (including `#` fragment) in `SecretResult`
- Work for both text and file secrets
- Pure client-side — no server involvement
- Minimal bundle size impact

**Non-Goals:**
- Custom QR code styling beyond basic dark mode adaptation
- Downloadable QR code image
- QR codes in any context other than `SecretResult`

## Decisions

### Library: `qrcode` (npm) over `react-qr-code`

`qrcode` (~14 KB min+gzip) can render to an `<svg>` element directly via `QRCode.toString(url, { type: 'svg' })` and has zero React dependencies. `react-qr-code` is a thin React wrapper around the same algorithm and adds ~1 KB — acceptable either way. We choose `qrcode` because it has broader adoption, active maintenance, and its SVG output can be set via `dangerouslySetInnerHTML` or rendered as an `<img src="data:...">` without coupling to React's rendering cycle.

The QR code is generated with `QRCode.toDataURL(url, { errorCorrectionLevel: 'M', margin: 2 })` producing a PNG data URL, used as an `<img>` src. This is simpler than managing SVG injection and works in all browsers.

### Placement: below the copy row, above the metadata

The QR code sits between the URL/copy row and the expiry metadata. This keeps the visual hierarchy logical: primary action (copy link) → alternative sharing method (QR) → contextual info (expiry, burn-after-read).

### Size: 160×160 px display

At 160 px the QR code is comfortably scannable from a typical desktop screen distance. The generated image is 256×256 px (default) and scaled via CSS to avoid blurriness on high-DPI screens.

### Dark mode: white module background

QR codes require high contrast. In dark mode the component renders a white-background QR code regardless of the page theme, with a subtle border to distinguish it from the card background.

## Risks / Trade-offs

- **Hash fragment in QR**: Browsers never send the `#` fragment to servers, but it is fully included in the string passed to the QR library — the library encodes the raw string, so the fragment is embedded in the QR data. Scanners decode the full URL including fragment correctly. No risk.
- **Bundle size**: `qrcode` adds ~14 KB gzipped. Acceptable for a sharing-focused app.
- **`dangerouslySetInnerHTML` not used**: Using a `<img>` data URL avoids any XSS surface. The URL being encoded is our own origin + hash, not user-supplied HTML.
