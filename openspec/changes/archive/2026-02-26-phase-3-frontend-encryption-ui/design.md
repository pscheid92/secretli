## Context

Phase 2 delivered the Go backend with secret CRUD endpoints. The frontend has placeholder pages (React 19 + Vite 6 + Tailwind CSS 4 + React Router 7 + TanStack Query v5). The old Vue frontend (`old/ui/src/libs/encryption.tsx`) implements the encryption protocol but uses the `url-safe-base64` npm package and lacks PBKDF2 password support. The Go CLI (`old/secretli/internal/encryption.go`) has the complete protocol including password support.

## Goals / Non-Goals

**Goals:**
- Port the encryption protocol to TypeScript with zero external dependencies (Web Crypto API only)
- Add PBKDF2-SHA512 password support (missing in old frontend, present in Go CLI)
- Build working SharePage and RetrievePage for full encrypt → share → retrieve → decrypt flow
- Ensure byte-level compatibility with Go's `base64.RawURLEncoding`
- Deletion token included in share URL for link holders to delete secrets

**Non-Goals:**
- File upload/download (Phase 4)
- User authentication UI (Phase 5)
- Dark mode, toast notifications, polished design (Phase 8)
- Vitest/Playwright tests (will add basic encryption unit tests only)

## Decisions

### 1. Zero-dep base64 implementation
**Choice:** Native `btoa`/`atob` with manual URL-safe character substitution (`+`→`-`, `/`→`_`, strip `=`).
**Why:** The old frontend used `url-safe-base64` npm package for this trivial transformation. Removing it means zero npm dependencies for the crypto layer, which is critical for a security-focused app.
**Alternative:** Keep the npm package — rejected because it adds supply chain risk for 10 lines of code.

### 2. PBKDF2 password path
**Choice:** When password is provided, derive a 32-byte master key via `PBKDF2-SHA512(password, share_secret_bytes, 100000, 32)`, then feed that into HKDF instead of the raw share secret. The share secret itself is preserved in the URL unchanged.
**Why:** Matches the Go CLI implementation exactly (`old/secretli/internal/encryption.go:38`). The old frontend lacked this feature.
**Note:** The `password_protected` flag is sent to the server so the RetrievePage knows to prompt for a password before attempting decryption.

### 3. Share URL format
**Choice:** `/s#<base64url_share_secret>` for secrets without deletion control, `/s#<base64url_share_secret>!<base64url_deletion_token>` when the creator wants the recipient to be able to delete.
**Why:** The `!` delimiter separates the share secret from the optional deletion token. The hash fragment is never sent to the server.

### 4. API client design
**Choice:** Thin typed wrapper around `fetch` with `credentials: "same-origin"`, throwing a typed `ApiError` on non-2xx responses. No axios, no abstraction layers.
**Why:** The API surface is small (3 endpoints for now). A typed wrapper provides autocomplete and error handling without a heavy dependency.

### 5. Component architecture
**Choice:** SharePage orchestrates SecretForm → encryption → API call → SecretResult. RetrievePage reads hash fragment → derives keys → API call → decryption → display.
**Why:** Simple page-level orchestration. No global state needed since the encryption flow is a one-shot operation.

## Risks / Trade-offs

- **[Web Crypto API availability]** → All modern browsers support it. No polyfill needed. Will fail clearly on ancient browsers.
- **[PBKDF2 100k iterations is slow]** → ~200ms on modern hardware, acceptable for a one-time operation. Shows loading state during derivation.
- **[Hash fragment not in server logs]** → By design (zero-knowledge). Debugging user issues is harder. Mitigation: clear client-side error messages.
- **[No encryption unit tests in this phase]** → Risk of subtle encoding mismatches with Go. Mitigation: manual round-trip testing with curl + browser; formal cross-language fixture tests in Phase 8.
