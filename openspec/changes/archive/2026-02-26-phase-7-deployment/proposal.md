## Why

The application is feature-complete and production-hardened but has no way to be built as a container, deployed to Kubernetes, or validated in CI. A Dockerfile, Helm chart, and CI pipeline are needed to go from "works on my machine" to a deployable artifact.

## What Changes

- **Dockerfile**: Multi-stage build (node:22-alpine → golang:1.23-alpine → gcr.io/distroless/static) producing a ~15MB image with the Go binary and embedded frontend
- **docker-compose.yml update**: Add the `app` service that builds from the Dockerfile, connects to Postgres and MinIO, and runs with appropriate environment variables
- **Helm chart**: Full Kubernetes deployment at `deploy/helm/secretli/` with deployment, service, ingress, configmap, secret, PDB, HPA, serviceaccount, and a pre-install/upgrade migration job
- **GitHub Actions CI/CD**: Workflow for lint, test, build, and Docker image push on push/PR

## Capabilities

### New Capabilities
- `dockerfile`: Multi-stage container build producing a minimal distroless image
- `helm-chart`: Kubernetes deployment via Helm with configurable values
- `ci-cd`: GitHub Actions workflow for automated lint, test, build, and image publish

### Modified Capabilities
- `dev-workflow`: docker-compose gains the `app` service for full local stack testing

## Impact

- **New files**: `Dockerfile`, `deploy/helm/secretli/` (Chart.yaml, values.yaml, values-production.yaml, templates/*), `.github/workflows/ci.yml`
- **Modified files**: `deploy/docker-compose.yml` (add app service)
- **No code changes**: All application code is unchanged; this is purely infrastructure
