# Contributing to Secretli

Thanks for your interest in contributing! Here's how to get started.

## Development Setup

Prerequisites: Go 1.27+, Node.js 24+, pnpm 10, Docker

```bash
# Clone the repo
git clone https://github.com/pscheid92/secretli.git
cd secretli

# Start Postgres and SeaweedFS
cd docker && docker compose -f docker-compose.dev.yml up -d && cd ..

# Configure environment
cp .env.example .env

# Start dev servers (migrations run automatically at backend startup)
make dev
```

## Running Tests

```bash
make test          # Full suite (requires Docker for testcontainers)
make test-short    # Fast unit tests only
make e2e           # Browser E2E tests against a running app
make e2e-large     # Opt-in large-file browser performance test
make lint          # Linters (Go + frontend)
make vuln          # govulncheck against Go dependencies
```

`make e2e-large` is not part of normal PR CI. It defaults to a near-limit synthetic file and can be tuned with `LARGE_E2E_SIZE_MB`, `LARGE_E2E_MAX_TOTAL_MS`, and `LARGE_E2E_MAX_HEAP_MIB`.

## Submitting Changes

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Add or update tests as needed
4. Ensure `make lint` and `make test` pass
5. Open a pull request against `main`

Keep pull requests focused — one feature or fix per PR.

## Reporting Issues

Use [GitHub Issues](https://github.com/pscheid92/secretli/issues) for bugs and feature requests. For security vulnerabilities, see [SECURITY.md](SECURITY.md).
