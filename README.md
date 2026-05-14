# Secretli

A zero-knowledge, end-to-end encrypted secret sharing platform. Share text and files securely with time-limited, self-destructing links.

Secrets are encrypted entirely in the browser — the server never sees plaintext data or encryption keys.

## Features

- **Zero-knowledge encryption** — XChaCha20-Poly1305 encryption happens client-side; the server stores only opaque blobs
- **Text and file sharing** — share secrets as text or upload files (up to 1 GiB)
- **Multi-file support** — select multiple files and store them as an encrypted random-access bundle
- **Burn after reading** — optionally destroy the secret after the first view
- **Password protection** — add a password for an extra layer of encryption (scrypt)
- **Configurable expiration** — from 5 minutes to 7 days
- **QR codes** — every share link includes a scannable QR code
- **Manual deletion** — owners can delete secrets before they expire
- **URL fragment security** — encryption keys live in the URL fragment (`#`), which is never sent to the server

## How It Works

1. The browser generates a random share secret and derives separate metadata keys, blob keys, public IDs, and access tokens using HKDF-SHA512
2. The secret (text or file) is encrypted with XChaCha20-Poly1305 (with AAD binding) and uploaded as an opaque blob; access and deletion tokens are stored only as SHA-256 hashes
3. A shareable link is created containing the keyset in the URL fragment (e.g., `/s#<shareSecret>`)
4. The recipient's browser derives the metadata token from the fragment, and derives the blob token from the password when password protection is enabled
5. The browser fetches the encrypted blob only after deriving the blob token, then decrypts it locally

The server only ever sees the public ID, metadata/blob access tokens, deletion token, and encrypted ciphertext in request handling, and persists token hashes rather than raw tokens. It never sees plaintext, passwords, or encryption keys.

## Quickstart

### Docker Compose

The fastest way to run everything locally:

```bash
cd docker
docker compose up -d
```

This starts the app, PostgreSQL, and SeaweedFS. The app is available at `http://localhost:8080`.

### Development Setup

Prerequisites: Go 1.26+, Node.js 24+, Docker (for Postgres and SeaweedFS)

```bash
# Start infrastructure
cd docker && docker compose -f docker-compose.dev.yml up -d && cd ..

# Configure environment
cp .env.example .env

# Run database migrations
go run . migrate

# Start dev servers (backend with hot-reload + frontend with Vite)
make dev
```

The frontend dev server runs at `http://localhost:5173` and proxies API requests to the Go backend on port 8080.

### Available Make Targets

```
make dev              # Run backend + frontend in dev mode
make build            # Production build (frontend + Go binary)
make test             # Full test suite (integration tests)
make test-short       # Fast unit tests only
make e2e              # Browser E2E tests against a running app
make e2e-large        # Opt-in large-file browser performance test
make lint             # Run Go and frontend linters
make clean            # Remove build artifacts
```

Large-file E2E is intentionally separate from PR CI. Run it against a local app with:

```bash
LARGE_E2E_SIZE_MB=1023 make e2e-large
```

It uploads and downloads a synthetic near-limit file, verifies the SHA-256 hash, and prints timing and browser heap samples. GitHub Actions also has a manual **Large E2E** workflow for this check.

## Tech Stack

**Backend:** Go, Echo, PostgreSQL, S3-compatible storage (SeaweedFS), Prometheus metrics

**Frontend:** React, TypeScript, Vite, Tailwind CSS, @noble/ciphers, @noble/hashes

## Configuration

Configuration is done via environment variables. See [`.env.example`](.env.example) for all options:

| Variable | Description | Default |
|---|---|---|
| `SERVER_PORT` | HTTP server port | `8080` |
| `DATABASE_URL` | PostgreSQL connection string | — |
| `S3_ENDPOINT` | S3-compatible object storage endpoint | — |
| `S3_BUCKET` | S3 bucket name | — |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | S3 credentials | — |
| `S3_USE_SSL` | Enable TLS for S3 | `false` |
| `MAX_FILE_SIZE` | Encrypted upload size limit in bytes | `1073741824` (1 GiB) |
| `CLEANUP_INTERVAL` | Expired secret cleanup frequency | `1m` |
| `ALLOWED_ORIGINS` | CORS allowed origins | — |
| `METRICS_TOKEN` | Optional bearer token required for `/metrics` | — |

## Deployment

The CI pipeline builds a minimal Docker image (distroless) and publishes it to GitHub Container Registry:

```
ghcr.io/pscheid92/secretli:main
ghcr.io/pscheid92/secretli:sha-<commit>
```

The application requires PostgreSQL and an S3-compatible object store. Health endpoints are available at `/api/v1/health/live` and `/api/v1/health/ready`.

## License

[MIT](LICENSE)
