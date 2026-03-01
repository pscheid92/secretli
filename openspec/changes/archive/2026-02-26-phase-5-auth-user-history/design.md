## Context

Phases 1-4 delivered anonymous secret sharing with text and file support. The Go backend has CRUD handlers, PostgreSQL storage, S3 file streaming, and a React frontend with client-side encryption. The config already defines `SESSION_MAX_AGE`, `COOKIE_DOMAIN`, and `COOKIE_SECURE` fields but they're unused. DESIGN.md specifies PostgreSQL-backed sessions (not JWT), bcrypt for passwords, and a `user_secrets` join table for history. The database has one migration (`001_create_secrets.sql`).

## Goals / Non-Goals

**Goals:**
- User registration with email + bcrypt password
- Server-side sessions stored in PostgreSQL with HttpOnly cookie
- Session middleware that makes the current user available to handlers
- Secret creation optionally links to authenticated user via `user_secrets`
- Paginated history endpoint showing a user's created secrets (metadata only)
- Frontend auth flow: register, login, logout, protected history page
- Anonymous secret creation continues to work without login

**Non-Goals:**
- OAuth / social login (future enhancement)
- Email verification or password reset (future enhancement)
- Admin panel or user management
- Rate limiting on auth endpoints (Phase 6)
- CORS configuration (Phase 6)

## Decisions

### 1. bcrypt cost 12 for password hashing
**Choice:** Use `golang.org/x/crypto/bcrypt` with cost 12 as specified in DESIGN.md.
**Why:** Cost 12 provides ~250ms hash time on modern hardware, balancing security and UX. Higher costs slow login unacceptably; lower costs are too fast for brute-force resistance.
**Alternative:** Argon2id — better resistance to GPU attacks, but bcrypt is simpler, well-supported in Go's extended stdlib, and sufficient for this use case.

### 2. Session token as random hex string
**Choice:** Generate 32 random bytes, hex-encode to 64-character string. Store as the session `id` primary key. Send as `session_id` cookie value.
**Why:** Matches DESIGN.md specification. Hex encoding is safe for cookies without URL encoding. 32 bytes provides 256 bits of entropy — infeasible to guess.
**Alternative:** UUID v4 — only 122 bits of entropy. Raw base64 — needs URL encoding in cookies.

### 3. Session lookup per request via middleware
**Choice:** Auth middleware reads `session_id` cookie, queries `sessions` table joined with `users`, attaches user to request context. Middleware is applied globally but does not reject unauthenticated requests — handlers decide whether auth is required.
**Why:** Most routes (secret create/retrieve) work for both authenticated and anonymous users. Only specific routes (history, `/me`) require authentication. The middleware just provides the user context; individual handlers check if a user is present.
**Alternative:** Apply auth middleware only to protected routes — less flexible, since secret creation needs optional auth.

### 4. `user_secrets` join table (not user_id on secrets)
**Choice:** Link users to secrets via a separate `user_secrets(user_id, secret_id, label)` join table rather than adding a `user_id` column to `secrets`.
**Why:** Matches DESIGN.md schema. Preserves the anonymity of secrets at the database level — the secrets table has no foreign key to users. The join is optional and only created when an authenticated user creates a secret. This also allows a `label` column for user-provided descriptions shown in history.
**Alternative:** Add nullable `user_id` to `secrets` — simpler queries but tighter coupling between anonymous secrets and user accounts.

### 5. History endpoint returns metadata only
**Choice:** `GET /api/v1/user/secrets?page=1&per_page=20` returns `public_id`, `label`, `secret_type`, `burn_after_read`, `expires_at`, `created_at`, `retrieved_at`, `password_protected`. Never returns `encrypted_data`, `nonce`, or tokens.
**Why:** The server should never expose encrypted content through the history endpoint. The user already has the share link if they need to access the secret. History is for management and status tracking.

### 6. Frontend AuthContext with TanStack Query
**Choice:** `AuthContext` provides `user`, `login()`, `logout()`, `register()`, and `isLoading`. Internally uses TanStack Query's `useQuery` for `/me` (on mount) and `useMutation` for login/register/logout. Context re-exports the query state so components can check `user` without calling hooks directly.
**Why:** TanStack Query handles caching, refetching, and loading states. The context wraps it in a clean API. No additional state library needed.
**Alternative:** Raw React state + fetch — loses caching, error handling, and loading state management.

### 7. Label field on secret creation
**Choice:** Add an optional `label` field to `CreateSecretRequest` and the file upload metadata. When authenticated, store it in `user_secrets.label`. When anonymous, ignore it.
**Why:** Labels help users identify secrets in their history (e.g., "AWS credentials for John"). The field is optional and backward-compatible.

## Risks / Trade-offs

- **[Session table growth]** → Sessions accumulate as users log in. Mitigation: the cleanup worker (Phase 6) will purge expired sessions. For now, sessions have `expires_at` and the query filters by it.
- **[No email verification]** → Users can register with any email. Mitigation: email is just an identifier for login, not used for communication. Verification can be added later without schema changes.
- **[Session per request DB query]** → Every request hits the sessions table. Mitigation: the query is indexed on session ID (primary key), so it's O(1). At expected traffic levels, this adds negligible latency (<1ms). An in-memory cache could be added later if needed.
- **[No CSRF protection]** → SameSite=Lax cookies provide baseline CSRF protection. All mutation endpoints use POST/DELETE (not GET). This is sufficient for the current threat model. Explicit CSRF tokens can be added in Phase 6 if needed.
