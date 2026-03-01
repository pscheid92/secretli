## Why

Secretli currently has no user accounts. Every secret is anonymous — once the share link is created, the creator has no way to manage their secrets beyond the deletion token embedded in the URL. Adding authentication enables users to track their shared secrets, see retrieval status, and manage expiration from a dashboard. This is the next logical step after the core sharing functionality (phases 1-4) is complete.

## What Changes

- Add `users` table with email + bcrypt password hashing
- Add `sessions` table for server-side session management (PostgreSQL-backed, not JWT)
- Add `user_secrets` join table linking authenticated users to their created secrets
- Add auth handlers: register, login, logout, get current user
- Add session middleware that reads session cookies and attaches user context to requests
- Modify secret creation handlers to optionally link secrets to the authenticated user
- Add user secret history endpoint with pagination
- Add frontend AuthContext for managing logged-in state
- Implement LoginPage, RegisterPage, and HistoryPage
- Update Layout navigation to show auth state and history link

## Capabilities

### New Capabilities
- `user-auth`: User registration with email/password (bcrypt), login with session creation, logout with session destruction, and `/me` endpoint for current user info
- `session-management`: Server-side PostgreSQL sessions with cookie-based auth, session middleware for route protection, session expiry and cleanup
- `user-history`: Paginated endpoint for listing a user's created secrets (metadata only, no encrypted data), with label support and retrieval status

### Modified Capabilities
- `go-server`: Add auth middleware to middleware chain, register auth and user routes
- `react-app`: Add AuthContext provider, protected route wrapper, update Layout with auth navigation
- `config`: Already has session config fields (SESSION_MAX_AGE, COOKIE_DOMAIN, COOKIE_SECURE) — no spec changes needed, just implementation
- `secret-crud`: Modify create handlers to accept optional session context and link secrets to authenticated users via `user_secrets`
- `api-client`: Add auth API functions (register, login, logout, me) and user secrets history endpoint

## Impact

- **Database**: 3 new migrations (users, sessions, user_secrets tables)
- **Go dependencies**: `golang.org/x/crypto` for bcrypt
- **Backend**: New handler, model, and store files for users and sessions; middleware additions
- **Frontend**: New pages (Login, Register, History), AuthContext, navigation updates
- **Existing endpoints**: Secret creation gains optional user linking (non-breaking — anonymous creation still works)
