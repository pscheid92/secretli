## 1. Base64 Utilities

- [x] 1.1 Create `web/frontend/src/lib/base64.ts` with `base64UrlEncode(bytes: Uint8Array): string` and `base64UrlDecode(str: string): Uint8Array` — zero-dep, using native `btoa`/`atob` with URL-safe character swap (`+`→`-`, `/`→`_`, strip `=`), matching Go's `base64.RawURLEncoding`

## 2. Encryption Module

- [x] 2.1 Create `web/frontend/src/lib/encryption.ts` with `KeySet` class: private constructor holding `shareSecret`, `encryptionKey`, `publicID`, `retrievalToken`, `deletionToken` as `Uint8Array` fields
- [x] 2.2 Implement `KeySet.generateRandom()`: generate 32 random bytes as share secret, derive keys via HKDF-SHA512, generate 16-byte random deletion token
- [x] 2.3 Implement `KeySet.fromShareSecret(encoded: string, password?: string)`: decode base64url share secret, optionally apply PBKDF2-SHA512(password, shareSecretBytes, 100000, 32), then derive keys via HKDF-SHA512
- [x] 2.4 Implement private `deriveKeys(keyBytes: Uint8Array)` helper: import key as HKDF, deriveBits with SHA-512 and info strings `"share_item_encryption_key"` (32B), `"share_item_uuid"` (16B), `"share_item_token"` (16B)
- [x] 2.5 Implement `KeySet.encrypt(plaintext: string)`: encode text as UTF-8, generate 12-byte random nonce, AES-256-GCM encrypt with derived encryption key, return `{ nonce: string, encrypted_data: string }` as base64url
- [x] 2.6 Implement `KeySet.decrypt(nonce: string, encryptedData: string)`: decode base64url inputs, AES-256-GCM decrypt, return UTF-8 string
- [x] 2.7 Implement `KeySet.getEncoded()`: return `{ shareSecret, publicID, retrievalToken, deletionToken }` as base64url strings
- [x] 2.8 Export `EncodedKeySet` and `EncryptedData` TypeScript interfaces

## 3. API Client

- [x] 3.1 Create `web/frontend/src/lib/api.ts` with `ApiError` class (status + message properties)
- [x] 3.2 Implement `createSecret(params)` — POST `/api/v1/secrets` with JSON body, returns `{ expires_at }`
- [x] 3.3 Implement `retrieveSecret(publicID, retrievalToken)` — POST `/api/v1/secrets/{publicID}` with `X-Retrieval-Token` header, returns `{ nonce, encrypted_data, secret_type, burn_after_read, password_protected }`
- [x] 3.4 Implement `deleteSecret(publicID, retrievalToken, deletionToken)` — DELETE `/api/v1/secrets/{publicID}` with both token headers

## 4. Components

- [x] 4.1 Create `web/frontend/src/components/ExpirationPicker.tsx` — select dropdown with options 5m/10m/15m/1h/4h/12h/1d/3d/7d, default `1d`, calls `onChange` with value string
- [x] 4.2 Create `web/frontend/src/components/SecretForm.tsx` — textarea, ExpirationPicker, burn-after-read checkbox, optional password input, submit button with loading state; calls `onSubmit({ text, expiration, burnAfterRead, password })`, validates non-empty text
- [x] 4.3 Create `web/frontend/src/components/SecretResult.tsx` — displays share URL in a read-only input, copy-to-clipboard button with "Copied!" feedback, shows expiration time and burn-after-read status

## 5. Pages

- [x] 5.1 Implement `SharePage.tsx` — renders SecretForm, on submit: generate KeySet (with optional password), encrypt text, call `createSecret()`, build share URL (`/s#<shareSecret>!<deletionToken>`), display SecretResult
- [x] 5.2 Implement `RetrievePage.tsx` — on mount: read `window.location.hash`, parse share secret and optional deletion token (split on `!`), derive KeySet, call `retrieveSecret()`, handle `password_protected` flag (show password prompt, re-derive with password), decrypt and display plaintext, show delete button if deletion token present
- [x] 5.3 Handle error states in RetrievePage: 404 (expired/not found), 403 (invalid token), wrong password (decryption failure), network errors

## 6. Layout and Navigation Update

- [x] 6.1 Update `Layout.tsx` — keep existing nav structure, ensure "Share" link goes to `/` and "File" link goes to `/file`

## 7. Verification

- [x] 7.1 Build frontend (`npm run build` in `web/frontend/`) and verify no TypeScript errors
- [ ] 7.2 Manual test: start dev servers, create a secret via SharePage, copy link, open in new tab, verify decrypted text matches
- [ ] 7.3 Manual test: create a password-protected secret, retrieve it, verify password prompt appears and decryption works with correct password
