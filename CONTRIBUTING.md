# Contributing to Secretli

Thanks for your interest in contributing! Here's how to get started.

## Development Setup

Prerequisites: Go 1.26+, Node.js 24+, Docker

```bash
# Clone the repo
git clone https://github.com/pscheid92/secretli.git
cd secretli

# Start Postgres and MinIO
cd docker && docker compose -f docker-compose.dev.yml up -d && cd ..

# Configure environment
cp .env.example .env

# Run database migrations
go run . migrate

# Start dev servers
make dev
```

## Running Tests

```bash
make test          # Full suite (requires Docker for testcontainers)
make test-short    # Fast unit tests only
make lint          # Linters (Go + frontend)
```

## Submitting Changes

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Add or update tests as needed
4. Ensure `make lint` and `make test` pass
5. Open a pull request against `main`

Keep pull requests focused — one feature or fix per PR.

## Reporting Issues

Use [GitHub Issues](https://github.com/pscheid92/secretli/issues) for bugs and feature requests. For security vulnerabilities, see [SECURITY.md](SECURITY.md).
