## 1. Database Migrations

- [x] 1.1 Add `golang.org/x/crypto` dependency (`go get golang.org/x/crypto`)
- [x] 1.2 Create `migrations/002_create_users.sql` with `users` table (id, email, password_hash, display_name, created_at, updated_at) and unique index on email
- [x] 1.3 Create `migrations/003_create_sessions.sql` with `sessions` table (id TEXT PK, user_id FK, expires_at, created_at) and indexes on user_id and expires_at
- [x] 1.4 Create `migrations/004_create_user_secrets.sql` with `user_secrets` table (user_id FK CASCADE, secret_id FK CASCADE, label, created_at) and composite PK

## 2. User Model and Repository

- [x] 2.1 Create `internal/model/user.go` with `User` struct (ID, Email, PasswordHash, DisplayName, CreatedAt, UpdatedAt) and request/response types
- [x] 2.2 Create `internal/store/user_repo.go` with `UserRepo` interface: `Create`, `GetByEmail`, `GetByID`
- [x] 2.3 Create `internal/store/user_repo_pg.go` implementing `UserRepo` with pgxpool — `Create` inserts user with bcrypt hash, `GetByEmail` for login, `GetByID` for session lookup

## 3. Session Model and Repository

- [x] 3.1 Create `internal/store/session_repo.go` with `SessionRepo` interface: `Create`, `GetByIDWithUser`, `Delete`, `DeleteExpiredSessions`
- [x] 3.2 Create `internal/store/session_repo_pg.go` implementing `SessionRepo` — `Create` generates 32 random bytes hex-encoded, `GetByIDWithUser` joins sessions+users and checks expiry, `Delete` removes session row

## 4. User Secrets Repository

- [x] 4.1 Add `UserSecretRepo` interface to `internal/store/user_secret_repo.go`: `LinkSecret(ctx, userID, secretID, label)`, `ListByUser(ctx, userID, page, perPage) ([]SecretSummary, total, error)`
- [x] 4.2 Create `internal/store/user_secret_repo_pg.go` implementing `UserSecretRepo` — `LinkSecret` inserts into user_secrets, `ListByUser` queries with pagination joining secrets for metadata

## 5. Auth Middleware

- [x] 5.1 Add auth context helpers in `internal/handler/auth_context.go`: `UserFromContext(ctx)` and `contextWithUser(ctx, user)` using `context.WithValue`
- [x] 5.2 Add session middleware in `internal/server/middleware.go`: read `session_id` cookie, call `SessionRepo.GetByIDWithUser`, attach user to context. Do not reject unauthenticated requests.
- [x] 5.3 Wire session middleware into the middleware chain in `server.go` (after logging, before route handling). Pass `SessionRepo` to middleware.

## 6. Auth Handlers

- [x] 6.1 Create `internal/handler/auth.go` with `AuthHandler` struct holding `UserRepo`, `SessionRepo`, and config fields (SessionMaxAge, CookieDomain, CookieSecure)
- [x] 6.2 Implement `Register` handler: validate fields, check email uniqueness, bcrypt hash (cost 12), create user, create session, set cookie, return user JSON
- [x] 6.3 Implement `Login` handler: find user by email, verify bcrypt, create session, set cookie, return user JSON. Return 401 for wrong email or password (same error message).
- [x] 6.4 Implement `Logout` handler: read session cookie, delete session from DB, clear cookie, return 204
- [x] 6.5 Implement `Me` handler: extract user from context, return 401 if absent, return user JSON

## 7. User History Handler

- [x] 7.1 Create `internal/handler/user.go` with `UserHandler` struct holding `UserSecretRepo`
- [x] 7.2 Implement `ListSecrets` handler: extract user from context (401 if absent), parse page/per_page query params (defaults 1/20), call `UserSecretRepo.ListByUser`, return paginated JSON response

## 8. Existing Handler Updates

- [x] 8.1 Add optional `label` field to `CreateSecretRequest` in `model/secret.go`
- [x] 8.2 Update `SecretHandler.CreateSecret` to accept `UserSecretRepo`, extract user from context, and call `LinkSecret` after successful secret creation (when authenticated)
- [x] 8.3 Update `FileHandler.UploadFile` to accept `UserSecretRepo`, extract user from context, and call `LinkSecret` after successful file upload (when authenticated). Add `label` to file metadata.
- [x] 8.4 Update `internal/server/routes.go` to register auth routes (`/api/v1/auth/*`) and user routes (`/api/v1/user/secrets`), pass repos to handlers

## 9. Server Wiring

- [x] 9.1 Update `internal/server/server.go` to create `UserRepo`, `SessionRepo`, `UserSecretRepo` from pgxpool and pass to route registration
- [x] 9.2 Update `server.New` to pass config session/cookie fields to auth handler construction

## 10. Frontend API Client

- [x] 10.1 Add `register(email, password, displayName)` to `lib/api.ts`
- [x] 10.2 Add `login(email, password)` to `lib/api.ts`
- [x] 10.3 Add `logout()` to `lib/api.ts`
- [x] 10.4 Add `getCurrentUser()` to `lib/api.ts`
- [x] 10.5 Add `getUserSecrets(page?, perPage?)` to `lib/api.ts`

## 11. Frontend Auth Context

- [x] 11.1 Create `web/frontend/src/context/AuthContext.tsx` with `AuthProvider` and `useAuth` hook — useQuery for `/me` on mount, useMutation for login/register/logout, expose `user`, `isLoading`, `login()`, `register()`, `logout()`
- [x] 11.2 Wrap app with `AuthProvider` in `App.tsx`

## 12. Frontend Pages

- [x] 12.1 Implement `LoginPage.tsx` — email/password form, call `useAuth().login()`, redirect to `/` on success, show error on failure, redirect if already logged in
- [x] 12.2 Implement `RegisterPage.tsx` — email/password/display_name form, call `useAuth().register()`, redirect to `/` on success, show error on failure, redirect if already logged in
- [x] 12.3 Implement `HistoryPage.tsx` — fetch `getUserSecrets()`, display paginated table with label, type, dates, status. Redirect to `/login` if unauthenticated.
- [x] 12.4 Update `Layout.tsx` navigation to show Login/Register links when unauthenticated, and History/display_name/Logout when authenticated (using `useAuth()`)

## 13. Tests

- [x] 13.1 Write auth handler unit tests: register (success, duplicate email, missing fields, short password), login (success, wrong password, wrong email), logout, me (authenticated, unauthenticated)
- [x] 13.2 Write user history handler unit tests: list secrets (success, unauthenticated, empty, pagination)
- [x] 13.3 Write secret creation tests verifying user_secrets linking (authenticated creates link, anonymous does not)

## 14. Verification

- [x] 14.1 Build Go backend (`go build .`) and verify no compilation errors
- [x] 14.2 Build frontend (`npm run build` in `web/frontend/`) and verify no TypeScript errors
- [x] 14.3 Run `go test ./...` and verify all tests pass
- [ ] 14.4 Manual test: register, login, create secrets (text + file), view history, logout, verify anonymous creation still works
