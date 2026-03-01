# Secretli - System Design Document

## 1. Overview

Secretli is a **zero-knowledge secret sharing service**. Users can share text secrets and files that are encrypted entirely in the browser before being sent to the server. The server never sees plaintext data or encryption keys.

This is a reboot of the original project (two separate repos: Go CLI + Vue 3 frontend) into a **single full-stack monorepo** with a modern stack.

---

## 2. Technology Stack

| Layer | Technology | Rationale |
|---|---|---|
| **Backend** | Go 1.23, stdlib `net/http` | Go 1.22+ has method-aware routing. No framework needed. |
| **Frontend** | React 19, TypeScript 5, Vite 6 | Modern, fast build tooling, strong typing. |
| **Styling** | Tailwind CSS 4 | Utility-first, small bundle, no component library lock-in. |
| **Database** | PostgreSQL (via `pgx/v5`) | Reliable, feature-rich. `pgx` is the fastest pure-Go driver. |
| **Object Storage** | S3-compatible / MinIO (via `minio-go/v7`) | Encrypted file blobs. MinIO for self-hosted, AWS S3 for cloud. |
| **Migrations** | `goose/v3` | Embeddable, supports SQL files, simple CLI. |
| **Auth** | Server-side sessions in PostgreSQL | Simpler than JWT, instantly revocable, no refresh token complexity. |
| **Frontend State** | TanStack Query v5 + React Context | Server state caching + auth context. No Redux needed. |
| **Routing** | React Router 7 | De facto standard, supports reading URL hash fragments. |
| **Deployment** | Single Go binary (embedded frontend), Kubernetes + Helm | One artifact to deploy. `embed.FS` bundles the React build. |

### Go Dependencies (minimal)

| Package | Purpose |
|---|---|
| `github.com/jackc/pgx/v5` | PostgreSQL driver + connection pooling (`pgxpool`) |
| `github.com/minio/minio-go/v7` | S3-compatible object storage client |
| `github.com/pressly/goose/v3` | Database migration tool |
| `golang.org/x/crypto` | bcrypt (user passwords) |
| `golang.org/x/time/rate` | Rate limiting |

Everything else is stdlib: `net/http`, `encoding/json`, `crypto/sha256`, `log/slog`, `embed`, `database/sql`.

---

## 3. Encryption Protocol

This implements the protocol described by 1Password for zero-trust sharing. **It must be preserved exactly from the old implementation.**

### Key Derivation

```
                                ┌──────────────────────┐
                                │  32 random bytes     │
                                │  (share secret)      │
                                └──────────┬───────────┘
                                           │
                        ┌──────────────────┼──── If password provided:
                        │                  │     PBKDF2-SHA512(password, share_secret, 100k, 32B)
                        │                  │     Result replaces share_secret as HKDF input
                        │                  ▼
                        │         ┌────────────────┐
                        │         │   HKDF-SHA512  │
                        │         │   (no salt)    │
                        │         └────────┬───────┘
                        │                  │
              ┌─────────┼──────────────────┼──────────────────┐
              │         │                  │                   │
              ▼         ▼                  ▼                   ▼
    ┌─────────────┐ ┌────────┐    ┌──────────────┐   ┌──────────────┐
    │ encryption  │ │ public │    │  retrieval   │   │  deletion    │
    │ key (32B)   │ │ ID(16B)│    │  token(16B)  │   │  token(16B)  │
    │             │ │        │    │              │   │  (random,    │
    │ info=       │ │ info=  │    │ info=        │   │  NOT derived)│
    │ "share_item │ │"share_ │    │"share_item_  │   │              │
    │ _encryption │ │ item_  │    │ token"       │   │              │
    │ _key"       │ │ uuid"  │    │              │   │              │
    └─────────────┘ └────────┘    └──────────────┘   └──────────────┘
         │               │               │                   │
         │               │               │                   │
    AES-256-GCM     Server lookup    Sent as header     Sent as header
    encrypt/decrypt  identifier      for retrieval      for deletion
```

### Protocol Details

1. **Generate** 32 random bytes → this is the **share secret**
2. **Optional password**: If provided, run `PBKDF2-SHA512(password, share_secret_bytes, 100000, 32)`. The output replaces the share secret as input to HKDF.
3. **Derive keys** via HKDF-SHA512 (no salt, info strings as labels):
   - `"share_item_encryption_key"` → 32 bytes → AES-256-GCM encryption key
   - `"share_item_uuid"` → 16 bytes → public ID for server-side lookup
   - `"share_item_token"` → 16 bytes → retrieval token
4. **Generate** 16 random bytes → **deletion token** (independently random, not derived)
5. **Encrypt** plaintext with AES-256-GCM using derived encryption key + random 12-byte nonce
6. **Encode** all binary values with URL-safe base64, no padding (`base64.RawURLEncoding` in Go)

### Zero-Knowledge Properties

- The **share secret** is placed in the URL fragment (`/s#<share_secret>`). URL fragments are never sent to the server by browsers.
- The server only receives: `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `encrypted_data`
- The server **cannot decrypt** the data — it never sees the share secret or encryption key
- The server stores `retrieval_token` and `deletion_token` as SHA-256 hashes for defense-in-depth

### File Encryption

Same protocol, applied to file contents:
- Read entire file into memory (up to 100MB limit)
- Encrypt with AES-256-GCM using the same derived encryption key + fresh random nonce
- Filename is also encrypted separately before sending to server
- Server streams encrypted blob to/from S3, never holding the full file in memory

---

## 4. Database Schema

### `secrets`

```sql
CREATE TABLE secrets (
    id                   BIGSERIAL PRIMARY KEY,
    public_id            TEXT NOT NULL UNIQUE,
    retrieval_token_hash TEXT NOT NULL,          -- SHA-256 of the token
    deletion_token_hash  TEXT NOT NULL,          -- SHA-256 of the token
    encrypted_data       TEXT,                   -- Base64 ciphertext (NULL for files)
    nonce                TEXT NOT NULL,          -- Base64 AES-GCM nonce
    secret_type          TEXT NOT NULL DEFAULT 'text'
                         CHECK (secret_type IN ('text', 'file')),
    storage_key          TEXT,                   -- S3 object key (for files)
    encrypted_filename   TEXT,                   -- Encrypted original filename
    encrypted_size       BIGINT,                -- Size of encrypted blob in bytes
    burn_after_read      BOOLEAN NOT NULL DEFAULT FALSE,
    password_protected   BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at           TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retrieved_at         TIMESTAMPTZ            -- Set on first retrieval
);

CREATE INDEX idx_secrets_public_id ON secrets (public_id);
CREATE INDEX idx_secrets_expires_at ON secrets (expires_at) WHERE retrieved_at IS NULL;
```

### `users`

```sql
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,                -- bcrypt (cost 12)
    display_name  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### `sessions`

```sql
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,               -- Random 32-byte token, hex-encoded
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);
```

### `user_secrets`

```sql
CREATE TABLE user_secrets (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret_id  BIGINT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
    label      TEXT NOT NULL DEFAULT '',        -- User-provided label for history
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, secret_id)
);
```

**Why SHA-256 for tokens (not bcrypt)?** The tokens are 16 bytes of cryptographically random data. Dictionary/brute-force attacks are infeasible. bcrypt's intentional slowness would add latency to every retrieval without security benefit.

---

## 5. API Design

All endpoints prefixed with `/api/v1`. The Go server also serves the React SPA at all non-API paths.

### Secret Endpoints

#### `POST /api/v1/secrets` — Create text secret

Request:
```json
{
  "public_id": "base64url",
  "retrieval_token": "base64url",
  "deletion_token": "base64url",
  "nonce": "base64url",
  "encrypted_data": "base64url",
  "expiration": "7d",
  "burn_after_read": false,
  "password_protected": false,
  "label": ""
}
```

Server hashes tokens with SHA-256 before storing. Converts expiration string (`5m`, `10m`, `15m`, `1h`, `4h`, `12h`, `1d`, `3d`, `7d`) to absolute timestamp. If session cookie is present, links secret to user via `user_secrets`.

Response: `201 Created`
```json
{ "expires_at": "2026-03-05T12:00:00Z" }
```

#### `POST /api/v1/secrets/{publicID}` — Retrieve text secret

Header: `X-Retrieval-Token: <base64url>`

Uses POST (not GET) because retrieval has side effects (burn-after-read, setting `retrieved_at`).

Server hashes the provided token and compares against stored hash using constant-time comparison. On match:
- If `burn_after_read`: return data, then delete the row (and S3 object if file)
- Otherwise: set `retrieved_at` if not already set, return data

Response: `200 OK`
```json
{
  "nonce": "base64url",
  "encrypted_data": "base64url",
  "secret_type": "text",
  "burn_after_read": false,
  "password_protected": false
}
```

#### `DELETE /api/v1/secrets/{publicID}` — Delete secret

Headers: `X-Retrieval-Token`, `X-Deletion-Token`

Server verifies both token hashes. On match: delete row and S3 object if applicable.

Response: `204 No Content`

### File Endpoints

#### `POST /api/v1/secrets/file` — Upload encrypted file

`multipart/form-data` with two parts:
- `metadata`: JSON with `public_id`, `retrieval_token`, `deletion_token`, `nonce`, `expiration`, `burn_after_read`, `password_protected`, `encrypted_filename`, `label`
- `file`: The encrypted binary blob

Server streams the file part directly to S3 (`secrets/{public_id}`). Never holds full file in memory. Max size: 100MB (enforced via `http.MaxBytesReader`).

Response: `201 Created`

#### `POST /api/v1/secrets/{publicID}/file` — Download encrypted file

Header: `X-Retrieval-Token`

Same auth as text retrieval. Server streams S3 object to response body. Returns `X-Encrypted-Filename` header.

Response: `200 OK` with `application/octet-stream` body

### Auth Endpoints

```
POST /api/v1/auth/register   { email, password, display_name } → 201 + Set-Cookie
POST /api/v1/auth/login      { email, password }               → 200 + Set-Cookie
POST /api/v1/auth/logout                                        → 204 + Clear-Cookie
GET  /api/v1/auth/me                                            → 200 { user } or 401
```

Cookie: `session_id=<hex>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=2592000`

### User Endpoints

```
GET /api/v1/user/secrets?page=1&per_page=20
```

Returns metadata only (no encrypted data): `public_id`, `label`, `secret_type`, `burn_after_read`, `expires_at`, `created_at`, `retrieved_at`, `password_protected`.

### Health Endpoints

```
GET /api/v1/health/live   → 200 "ok"
GET /api/v1/health/ready  → 200 "ok" (checks DB + S3 connectivity)
```

---

## 6. Project Structure

```
secretli/
├── go.mod
├── go.sum
├── main.go                             # Entry point
├── Makefile
├── Dockerfile
├── .air.toml                           # Go hot-reload config
│
├── cmd/
│   └── server.go                       # CLI commands: serve, migrate
│
├── internal/
│   ├── config/
│   │   └── config.go                   # Env var loading
│   ├── server/
│   │   ├── server.go                   # HTTP server setup, SPA handler
│   │   ├── middleware.go               # Logging, recovery, CORS, rate limit, auth, security headers
│   │   └── routes.go                   # Route registration
│   ├── handler/
│   │   ├── secret.go                   # Create/retrieve/delete text secrets
│   │   ├── file.go                     # Upload/download encrypted files
│   │   ├── auth.go                     # Register/login/logout/me
│   │   ├── user.go                     # User secret history
│   │   └── health.go                   # Liveness/readiness probes
│   ├── model/
│   │   ├── secret.go                   # Secret domain types
│   │   └── user.go                     # User + session domain types
│   ├── store/
│   │   ├── postgres.go                 # pgxpool connection setup
│   │   ├── secret_repo.go             # Secret CRUD + DeleteExpired
│   │   ├── user_repo.go               # User CRUD
│   │   └── session_repo.go            # Session CRUD
│   ├── storage/
│   │   └── s3.go                       # MinIO/S3: streaming put/get/delete
│   ├── crypto/
│   │   └── hash.go                     # SHA-256 token hashing, constant-time compare
│   └── cleanup/
│       └── worker.go                   # Background goroutine: expire secrets + cleanup S3
│
├── migrations/
│   ├── 001_create_secrets.up.sql
│   ├── 001_create_secrets.down.sql
│   ├── 002_create_users.up.sql
│   ├── 002_create_users.down.sql
│   ├── 003_create_sessions.up.sql
│   ├── 003_create_sessions.down.sql
│   ├── 004_create_user_secrets.up.sql
│   └── 004_create_user_secrets.down.sql
│
├── web/
│   ├── embed.go                        # //go:embed frontend/dist/*
│   └── frontend/                       # React application
│       ├── package.json
│       ├── tsconfig.json
│       ├── vite.config.ts
│       ├── tailwind.config.ts
│       ├── postcss.config.js
│       ├── index.html
│       └── src/
│           ├── main.tsx
│           ├── App.tsx
│           ├── index.css               # Tailwind directives
│           ├── lib/
│           │   ├── encryption.ts       # Ported encryption protocol (Web Crypto API)
│           │   ├── base64.ts           # Zero-dep URL-safe base64
│           │   └── api.ts              # Typed fetch wrapper
│           ├── hooks/
│           │   ├── useAuth.ts
│           │   └── useSecret.ts
│           ├── context/
│           │   └── AuthContext.tsx
│           ├── pages/
│           │   ├── SharePage.tsx
│           │   ├── RetrievePage.tsx
│           │   ├── FilePage.tsx
│           │   ├── LoginPage.tsx
│           │   ├── RegisterPage.tsx
│           │   ├── HistoryPage.tsx
│           │   └── NotFoundPage.tsx
│           └── components/
│               ├── Layout.tsx          # Shell: nav + footer
│               ├── SecretForm.tsx      # Text input, options
│               ├── SecretResult.tsx    # Share link, copy button
│               ├── FileUpload.tsx      # Drag-and-drop
│               └── ExpirationPicker.tsx
│
└── deploy/
    ├── docker-compose.yml              # Local dev: app + Postgres + MinIO
    └── helm/
        └── secretli/
            ├── Chart.yaml
            ├── values.yaml
            ├── values-production.yaml
            └── templates/
                ├── _helpers.tpl
                ├── deployment.yaml
                ├── service.yaml
                ├── ingress.yaml
                ├── configmap.yaml
                ├── secret.yaml
                ├── pdb.yaml
                ├── hpa.yaml
                ├── serviceaccount.yaml
                └── job-migrate.yaml    # Pre-install/upgrade hook
```

---

## 7. Frontend Architecture

### Routing

```
/              → SharePage         (create a text secret)
/file          → FilePage          (upload an encrypted file)
/s#<secret>    → RetrievePage      (retrieve + decrypt using hash fragment)
/login         → LoginPage
/register      → RegisterPage
/history       → HistoryPage       (authenticated, user's secret history)
*              → NotFoundPage
```

The share URL format is `https://domain.com/s#<shareSecret>`. The hash fragment never reaches the server.

### State Management

- **TanStack Query v5** for all server state (API calls, caching, loading/error states)
- **React Context** for auth state only (`AuthContext` provides current user + login/logout)
- No Redux, Zustand, or other global state library

### Encryption Module (`lib/encryption.ts`)

Ported from `old/ui/src/libs/encryption.tsx` with these changes:
- **Zero-dependency base64**: Replace `url-safe-base64` npm package with native `btoa`/`atob` + manual URL-safe character swap (`+`→`-`, `/`→`_`, strip `=`)
- **Password support**: Add PBKDF2-SHA512 path (the old frontend lacked this; the CLI had it)
- **File encryption**: `encryptFile(file: File)` and `decryptFile(blob, nonce)` methods on KeySet
- **Filename encryption**: Encrypt filename before sending to server

### API Client (`lib/api.ts`)

Thin typed wrapper around `fetch` with `credentials: "same-origin"` for session cookies. Custom `ApiError` class with status code + message.

---

## 8. Backend Architecture Details

### Configuration (`internal/config/config.go`)

All via environment variables:

| Variable | Default | Required |
|---|---|---|
| `SERVER_PORT` | `8080` | No |
| `DATABASE_URL` | — | Yes |
| `S3_ENDPOINT` | — | Yes |
| `S3_BUCKET` | `secretli` | No |
| `S3_ACCESS_KEY` | — | Yes |
| `S3_SECRET_KEY` | — | Yes |
| `S3_USE_SSL` | `true` | No |
| `S3_REGION` | `us-east-1` | No |
| `MAX_FILE_SIZE` | `104857600` (100MB) | No |
| `CLEANUP_INTERVAL` | `1m` | No |
| `SESSION_MAX_AGE` | `720h` (30d) | No |
| `COOKIE_DOMAIN` | `""` | No |
| `COOKIE_SECURE` | `true` | No |
| `ALLOWED_ORIGINS` | `""` (same-origin) | No |

### HTTP Server

```go
// Go 1.22+ method-aware routing
mux.HandleFunc("POST /api/v1/secrets", h.CreateSecret)
mux.HandleFunc("POST /api/v1/secrets/file", h.UploadFile)       // Must register before {publicID}
mux.HandleFunc("POST /api/v1/secrets/{publicID}", h.RetrieveSecret)
mux.HandleFunc("POST /api/v1/secrets/{publicID}/file", h.DownloadFile)
mux.HandleFunc("DELETE /api/v1/secrets/{publicID}", h.DeleteSecret)
// ... auth, user, health routes
```

Middleware chain: recovery → request ID → structured logging (`slog`) → CORS → rate limit → security headers → auth (optional)

### SPA Handler

Serves the embedded React build. For any path not matching a static file, serves `index.html` (React Router handles client-side routing).

```go
func spaHandler(fsys fs.FS) http.Handler {
    fileServer := http.FileServer(http.FS(fsys))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        f, err := fsys.Open(strings.TrimPrefix(r.URL.Path, "/"))
        if err != nil {
            // File not found → serve index.html for SPA routing
            r.URL.Path = "/"
        } else {
            f.Close()
        }
        fileServer.ServeHTTP(w, r)
    })
}
```

### Token Hashing

```go
func HashToken(token string) string {
    decoded, _ := base64.RawURLEncoding.DecodeString(token)
    h := sha256.Sum256(decoded)
    return hex.EncodeToString(h[:])
}

func VerifyToken(token, storedHash string) bool {
    return subtle.ConstantTimeCompare([]byte(HashToken(token)), []byte(storedHash)) == 1
}
```

### Cleanup Worker

Background goroutine running every `CLEANUP_INTERVAL`:

```sql
SELECT id, storage_key FROM secrets WHERE expires_at < NOW() LIMIT 500 FOR UPDATE SKIP LOCKED
```

Deletes S3 objects for file-type secrets, then batch deletes rows. `SKIP LOCKED` is safe for multiple replicas.

### Rate Limiting

In-memory token bucket per IP via `golang.org/x/time/rate`:
- Secret creation: 10/min
- Secret retrieval: 30/min
- Auth endpoints: 5/min (brute-force protection)
- File upload: 5/min

### Security Headers

```
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'
Referrer-Policy: no-referrer
```

`no-referrer` is critical: prevents URL fragments from leaking via the Referer header.

---

## 9. Deployment

### Dockerfile (multi-stage)

```
Stage 1: node:22-alpine     → Build React frontend (npm ci + npm run build)
Stage 2: golang:1.23-alpine → Build Go binary with embedded frontend
Stage 3: distroless/static  → Final image (~15MB), no shell, minimal attack surface
```

### Docker Compose (local development)

```yaml
services:
  app:         # Go server on :8080
  postgres:    # PostgreSQL on :5432
  minio:       # MinIO on :9000 (console :9001)
```

### Helm Chart

- **deployment.yaml**: Go binary with `serve` command, env from ConfigMap + Secret, liveness/readiness probes
- **job-migrate.yaml**: Pre-install/upgrade hook running `secretli migrate up`
- **service.yaml**: ClusterIP on port 80 → 8080
- **ingress.yaml**: Configurable for nginx/traefik ingress controllers
- **configmap.yaml**: Non-sensitive config
- **secret.yaml**: DATABASE_URL, S3 credentials (or reference external Secret)
- **pdb.yaml**: PodDisruptionBudget (minAvailable: 1)
- **hpa.yaml**: Optional HorizontalPodAutoscaler

---

## 10. Implementation Phases

### Phase 1: Project Scaffold
Set up monorepo structure: Go module, React app (Vite + TS + Tailwind), `embed.go`, basic HTTP server with SPA handler and health endpoints, Makefile, Air config, Vite proxy. **Verify:** `make dev` runs both servers, frontend loads, health check responds.

### Phase 2: Database + Text Secret CRUD
SQL migrations, goose integration, pgxpool setup, secret repository, SHA-256 token hashing, create/retrieve/delete handlers, expiration parsing, burn-after-read. **Verify:** Full create→retrieve→delete cycle via curl.

### Phase 3: Frontend Encryption + Secret Sharing UI
Port `encryption.ts` + `base64.ts` (zero-dep), add PBKDF2 password support, build SharePage, RetrievePage, SecretForm, SecretResult, Layout, React Router setup. **Verify:** Create secret in browser, copy share link, open in new tab, see decrypted text. With and without password.

### Phase 4: File Upload/Download
S3 client (streaming), file upload/download handlers, MaxBytesReader, FileUpload component, file encrypt/decrypt in KeySet, FilePage, extend RetrievePage for files. **Verify:** Upload file, retrieve, verify decrypted output matches original.

### Phase 5: Authentication + User History
User/session repos, auth handlers, session middleware, user_secrets linking, AuthContext, LoginPage, RegisterPage, HistoryPage. **Verify:** Register, login, create secrets, see history, logout.

### Phase 6: Production Hardening
Cleanup worker, rate limiting, security headers, CORS, graceful shutdown. **Verify:** Expired secrets cleaned up, rate limits trigger 429s.

### Phase 7: Deployment
Dockerfile, Helm chart, docker-compose.yml, GitHub Actions CI/CD. **Verify:** `docker build` works, Helm deploys to cluster.

### Phase 8: Polish
Toast notifications, loading states, form validation, copy-to-clipboard, dark mode, responsive design, Playwright e2e tests.

---

## 11. Testing Strategy

### Backend (Go)
- **Unit tests**: Handlers via `httptest.NewRecorder` with interface-based mocks for store layer
- **Integration tests**: Real PostgreSQL via `testcontainers-go` for store layer
- **Crypto tests**: Verify token hashing, constant-time comparison, known SHA-256 outputs

### Frontend (React)
- **Unit tests**: Encryption module with Vitest (round-trip encrypt/decrypt, password paths, base64 encoding matches Go's `RawURLEncoding`)
- **Component tests**: Vitest + React Testing Library
- **E2E tests**: Playwright for full share-and-retrieve flow

### Cross-Language Compatibility
Fixture-based test: encrypt known plaintext with known key in Go → verify TypeScript decrypts correctly (and vice versa). Ensures the protocol implementations are byte-compatible.
