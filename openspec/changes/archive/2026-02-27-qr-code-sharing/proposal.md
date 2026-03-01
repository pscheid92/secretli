## Why

Sharing a secret URL with a mobile device is awkward — copying a long URL with a hash fragment and sending it via a separate channel adds friction. A QR code lets users instantly scan the share link with any phone camera, making mobile sharing seamless.

## What Changes

- Add a QR code display to the `SecretResult` component showing the full share URL (including the `#` fragment containing the share secret)
- The QR code appears immediately after a secret is created, alongside the existing copy-to-clipboard link
- Works for both text and file secrets (wherever `SecretResult` is rendered)
- QR code is rendered entirely client-side using a lightweight library — no server involvement

## Capabilities

### New Capabilities
- `qr-code`: Client-side QR code generation and display in the SecretResult component

### Modified Capabilities
- `secret-sharing-ui`: SecretResult component gains a QR code display section

## Impact

- `web/frontend/src/components/SecretResult.tsx` — add QR code rendering
- New npm dependency: a client-side QR code library (e.g., `qrcode` or `react-qr-code`)
- No backend changes
- No API changes
