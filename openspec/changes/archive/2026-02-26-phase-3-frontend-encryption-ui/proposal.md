## Why

The backend can create, retrieve, and delete encrypted secrets (Phase 2), but users have no way to interact with the service through a browser. We need to port the zero-knowledge encryption protocol to TypeScript (Web Crypto API) and build the core secret-sharing UI so users can create encrypted secrets, get a share link, and retrieve/decrypt them — all client-side.

## What Changes

- Port the encryption protocol from `old/ui/src/libs/encryption.tsx` to a new `lib/encryption.ts` using Web Crypto API
- Replace the `url-safe-base64` npm dependency with a zero-dep `lib/base64.ts` using native `btoa`/`atob` + URL-safe character swap (matching Go's `base64.RawURLEncoding`)
- Add PBKDF2-SHA512 password support (from the Go CLI implementation, missing in the old frontend)
- Create a typed API client (`lib/api.ts`) wrapping `fetch` with proper error handling
- Build SharePage with SecretForm (textarea, expiration picker, burn-after-read toggle, optional password) and SecretResult (share link with copy button)
- Build RetrievePage that reads the URL hash fragment, derives keys, calls the API, decrypts, and displays the secret (with password prompt if needed)
- Update Layout navigation and React Router configuration

## Capabilities

### New Capabilities
- `encryption`: Zero-knowledge encryption protocol — HKDF-SHA512 key derivation, AES-256-GCM encrypt/decrypt, PBKDF2 password support, URL-safe base64 encoding
- `secret-sharing-ui`: SharePage (create + encrypt + store), RetrievePage (retrieve + decrypt + display), SecretForm, SecretResult, ExpirationPicker components
- `api-client`: Typed fetch wrapper for all backend API calls with error handling

### Modified Capabilities

## Impact

- **Frontend files**: `web/frontend/src/lib/` (new), `web/frontend/src/pages/SharePage.tsx`, `web/frontend/src/pages/RetrievePage.tsx`, `web/frontend/src/components/` (new and modified)
- **No backend changes**: All encryption happens client-side; backend API is unchanged from Phase 2
- **No new npm dependencies**: Web Crypto API is built into browsers; base64 is zero-dep
- **Browser compatibility**: Requires Web Crypto API (all modern browsers)
