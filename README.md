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
2. Text and files alike are packed into one bundle: each 4 MiB record is encrypted with XChaCha20-Poly1305 (with AAD binding to its position), followed by an encrypted manifest and a small footer
3. The bundle is streamed to the server as multipart parts and reassembled in object storage, so neither side ever holds the whole thing in memory; access and deletion tokens are stored only as SHA-256 hashes
4. A shareable link is created containing the keyset in the URL fragment (e.g., `/s#<shareSecret>`)
5. The recipient's browser derives the metadata token from the fragment, and derives the blob token from the password when password protection is enabled
6. After starting a retrieval session, the browser reads the manifest and fetches only the byte ranges it needs, decrypting each record locally

The server only ever sees the public ID, metadata/blob access tokens, deletion token, and encrypted ciphertext in request handling, and persists token hashes rather than raw tokens. It never sees plaintext, passwords, or encryption keys.

## Quickstart

### Docker Compose

The fastest way to run everything locally:

```bash
docker compose -f docker/docker-compose.yml --profile app up -d
```

This starts the app, PostgreSQL, and SeaweedFS. The app is available at `http://localhost:8080`. Without `--profile app` you get just the two backing services, which is what you want when running the app from source.

### Development Setup

Prerequisites: Go 1.27+, Node.js 24+, pnpm 10, Docker (for Postgres and SeaweedFS)

```bash
# Start infrastructure
docker compose -f docker/docker-compose.yml up -d

# Configure environment
cp .env.example .env

# Start dev servers (backend with hot-reload + frontend with Vite).
# Database migrations run automatically when the backend starts.
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
| `S3_ENDPOINT` | S3-compatible object storage endpoint (host:port or URL) | — |
| `S3_BUCKET` | S3 bucket name (must already exist) | `secretli` |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | S3 credentials | — |
| `S3_USE_SSL` | Use HTTPS for a bare host:port endpoint | `true` |
| `S3_REGION` | Region used for request signing | `us-east-1` |
| `MAX_FILE_SIZE` | Encrypted upload size limit in bytes; every secret is streamed as S3 multipart parts of up to 32 MiB | `1073741824` (1 GiB) |
| `CLEANUP_INTERVAL` | Expired secret cleanup frequency | `1m` |
| `ALLOWED_ORIGINS` | CORS allowed origins | — |
| `METRICS_TOKEN` | Optional bearer token required for `/metrics` | — |
| `TRUSTED_PROXIES` | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is trusted for client IP resolution and rate limiting. Leave empty when clients connect directly. | — |

## Deployment

The CI pipeline builds a minimal Docker image (distroless) and publishes it to GitHub Container Registry:

```
ghcr.io/pscheid92/secretli:main
ghcr.io/pscheid92/secretli:sha-<commit>
```

Images are built for `linux/amd64` and `linux/arm64`, carry an SBOM and SLSA provenance attestation, and are signed with [cosign](https://github.com/sigstore/cosign) (keyless, via GitHub OIDC). Verify a tag with:

```bash
cosign verify ghcr.io/pscheid92/secretli:main \
  --certificate-identity-regexp 'https://github.com/pscheid92/secretli/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The application requires PostgreSQL and an S3-compatible object store. Migrations run automatically at startup. Health endpoints are available at `/api/v1/health/live` and `/api/v1/health/ready`.

When the app sits behind a reverse proxy, set `TRUSTED_PROXIES` to the proxy's IP or CIDR so rate limits key on the real client address; without it, forwarded headers are ignored.

## License

[MIT](LICENSE)
