## 1. Dockerfile

- [x] 1.1 Create `Dockerfile` with three stages: node:22-alpine (frontend build), golang:1.23-alpine (Go build with CGO_ENABLED=0), gcr.io/distroless/static-debian12 (final image with only the binary)
- [x] 1.2 Add `.dockerignore` to exclude node_modules, tmp, bin, .git, and other unnecessary files
- [x] 1.3 Verify Docker build succeeds: `docker build -t secretli:dev .`

## 2. Docker Compose

- [x] 2.1 Update `deploy/docker-compose.yml` to add `app` service that builds from the Dockerfile, depends on postgres and minio, exposes port 8080, and configures all required environment variables

## 3. Helm Chart Structure

- [x] 3.1 Create `deploy/helm/secretli/Chart.yaml` with chart metadata (name, version, appVersion, description)
- [x] 3.2 Create `deploy/helm/secretli/values.yaml` with default configuration (replicas, image, ports, env vars, resource limits, ingress disabled, HPA disabled)
- [x] 3.3 Create `deploy/helm/secretli/templates/_helpers.tpl` with standard template helpers (name, fullname, labels, selectorLabels)

## 4. Helm Chart Templates

- [x] 4.1 Create `templates/serviceaccount.yaml`
- [x] 4.2 Create `templates/configmap.yaml` with non-sensitive env vars (SERVER_PORT, S3_BUCKET, S3_REGION, S3_USE_SSL, CLEANUP_INTERVAL, SESSION_MAX_AGE, COOKIE_SECURE, ALLOWED_ORIGINS, MAX_FILE_SIZE)
- [x] 4.3 Create `templates/secret.yaml` with sensitive env vars (DATABASE_URL, S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, COOKIE_DOMAIN)
- [x] 4.4 Create `templates/deployment.yaml` with container config, env from ConfigMap+Secret, liveness/readiness probes, resource limits, serviceaccount reference
- [x] 4.5 Create `templates/service.yaml` (ClusterIP, port 80 → 8080)
- [x] 4.6 Create `templates/ingress.yaml` (conditional on ingress.enabled)
- [x] 4.7 Create `templates/pdb.yaml` (PodDisruptionBudget, minAvailable: 1)
- [x] 4.8 Create `templates/hpa.yaml` (conditional on autoscaling.enabled)
- [x] 4.9 Create `templates/job-migrate.yaml` (pre-install/pre-upgrade hook, hook-delete-policy: before-hook-creation)

## 5. Production Values

- [x] 5.1 Create `deploy/helm/secretli/values-production.yaml` with production overrides (replicas >= 2, resource limits, ingress enabled with TLS)

## 6. GitHub Actions CI

- [x] 6.1 Create `.github/workflows/ci.yml` with lint job (go vet, frontend eslint), test job (go test, frontend tests), and build job (docker build, push on main only)

## 7. Verification

- [x] 7.1 Verify `docker build` succeeds
- [x] 7.2 Verify `helm lint deploy/helm/secretli/` passes
- [x] 7.3 Verify `helm template deploy/helm/secretli/` renders valid YAML
