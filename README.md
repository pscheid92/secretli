# Secretli

A zero-knowledge, end-to-end encrypted secret sharing platform. Share text and files securely with time-limited, self-destructing links.

Secrets are encrypted entirely in the browser — the server never sees plaintext data or encryption keys.

## Features

- **Zero-knowledge encryption** — AES-256-GCM encryption happens client-side; the server stores only opaque blobs
- **Text and file sharing** — share secrets as text or upload files (up to 100 MB)
- **Multi-file support** — select multiple files and they're automatically zipped client-side
- **Burn after reading** — optionally destroy the secret after the first view
- **Password protection** — add a password for an extra layer of encryption (PBKDF2, 210k iterations)
- **Configurable expiration** — from 5 minutes to 7 days
- **QR codes** — every share link includes a scannable QR code
- **Manual deletion** — owners can delete secrets before they expire
- **URL fragment security** — encryption keys live in the URL fragment (`#`), which is never sent to the server

## How It Works

1. The browser generates a random keyset and derives an encryption key, public ID, and retrieval token using HKDF-SHA512
2. The secret (text or file) is encrypted with AES-256-GCM and uploaded as an opaque blob
3. A shareable link is created containing the keyset in the URL fragment (e.g., `/s#<shareSecret>`)
4. The recipient's browser derives the same keys from the fragment, fetches the encrypted blob, and decrypts it locally

The server only ever sees the public ID, retrieval token, and encrypted ciphertext — never the plaintext or encryption key.

## Quickstart

### Docker Compose

The fastest way to run everything locally:

```bash
cd docker
docker compose up -d
```

This starts the app, PostgreSQL, and MinIO. The app is available at `http://localhost:8080`.

### Development Setup

Prerequisites: Go 1.26+, Node.js 24+, Docker (for Postgres and MinIO)

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
make lint             # Run Go and frontend linters
make clean            # Remove build artifacts
```

## Tech Stack

**Backend:** Go, Echo, PostgreSQL, S3-compatible storage (MinIO), Prometheus metrics

**Frontend:** React, TypeScript, Vite, Tailwind CSS, Web Crypto API

## Configuration

Configuration is done via environment variables. See [`.env.example`](.env.example) for all options:

| Variable | Description | Default |
|---|---|---|
| `SERVER_PORT` | HTTP server port | `8080` |
| `DATABASE_URL` | PostgreSQL connection string | — |
| `S3_ENDPOINT` | S3/MinIO endpoint | — |
| `S3_BUCKET` | S3 bucket name | — |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | S3 credentials | — |
| `S3_USE_SSL` | Enable TLS for S3 | `false` |
| `MAX_FILE_SIZE` | Upload size limit in bytes | `104857600` (100 MB) |
| `CLEANUP_INTERVAL` | Expired secret cleanup frequency | `1m` |
| `ALLOWED_ORIGINS` | CORS allowed origins | — |

## Deployment

The CI pipeline builds a minimal Docker image (distroless) and publishes it to GitHub Container Registry:

```
ghcr.io/pscheid92/secretli:main
ghcr.io/pscheid92/secretli:<version>
ghcr.io/pscheid92/secretli:sha-<commit>
```

The application requires PostgreSQL and an S3-compatible object store. Health endpoints are available at `/api/v1/health/live` and `/api/v1/health/ready`.

## License

[MIT](LICENSE)
