## Context

Phases 1–6 delivered a complete, production-hardened application. The codebase builds locally via `make build` (frontend + Go binary with embedded SPA). Docker-compose provides Postgres and MinIO for local dev but has no app service. There is no Dockerfile, no Kubernetes manifests, and no CI pipeline.

The DESIGN.md specifies:
- Multi-stage Dockerfile: node:22-alpine → golang:1.23-alpine → distroless/static
- Helm chart at `deploy/helm/secretli/` with deployment, service, ingress, configmap, secret, PDB, HPA, serviceaccount, and a pre-install/upgrade migration job
- Docker-compose with app + postgres + minio services

## Goals / Non-Goals

**Goals:**
- Produce a single minimal container image (~15MB) containing the Go binary with embedded frontend
- Provide a Helm chart for Kubernetes deployment with all necessary templates
- Add the app service to docker-compose for full local stack testing
- Set up GitHub Actions CI for lint, test, build, and Docker image push

**Non-Goals:**
- Production-grade CI/CD with staging environments, canary deploys, or blue/green
- Terraform/Pulumi infrastructure provisioning
- Container registry setup (assume ghcr.io / Docker Hub exists)
- Secrets management integration (Vault, External Secrets Operator)
- ArgoCD or GitOps delivery

## Decisions

### 1. Three-stage Dockerfile

Stage 1 (node:22-alpine): `npm ci && npm run build` — produces `web/frontend/dist/`.
Stage 2 (golang:1.23-alpine): copies `dist/` into place, runs `CGO_ENABLED=0 go build` — produces the binary.
Stage 3 (gcr.io/distroless/static-debian12): copies only the binary — minimal attack surface, no shell.

**Why distroless over scratch?** Distroless includes CA certificates and timezone data, which are needed for HTTPS S3 connections and timestamp handling without extra `COPY` steps.

### 2. Helm chart structure follows DESIGN.md exactly

Templates: deployment, service, ingress, configmap, secret, PDB, HPA, serviceaccount, job-migrate. Two values files: `values.yaml` (defaults, local/dev) and `values-production.yaml` (overrides for production).

**Why a migration job instead of init container?** The DESIGN.md specifies a pre-install/upgrade hook job. This is better than an init container because it runs once per release, not once per pod, avoiding migration race conditions with multiple replicas.

### 3. GitHub Actions CI workflow

Single workflow file `.github/workflows/ci.yml` with jobs:
- **lint**: `go vet`, `golangci-lint`, frontend ESLint
- **test**: Go tests, frontend tests (Vitest)
- **build**: Docker build + push (only on main branch push, not PRs)

**Why single workflow over multiple?** Simpler to maintain. Jobs within the workflow run in parallel where possible.

### 4. Docker-compose app service builds from Dockerfile

The app service uses `build: .` to build from the Dockerfile, connects to the postgres and minio services via Docker networking, and exposes port 8080.

## Risks / Trade-offs

- **Distroless has no shell for debugging** → Use `kubectl debug` with an ephemeral container if needed, or temporarily switch to alpine base for troubleshooting
- **Helm chart is opinionated** → Values files provide escape hatches for most configuration; templates use standard patterns
- **CI builds Docker image on every main push** → Cost is low; can add tag-based triggers later if needed
- **Migration job could fail and block deployment** → Helm hook has `hook-delete-policy: before-hook-creation` to clean up failed jobs; rollback is manual via `goose down`
