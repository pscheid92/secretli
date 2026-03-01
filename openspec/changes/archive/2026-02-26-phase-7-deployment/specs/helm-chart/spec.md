## ADDED Requirements

### Requirement: Helm chart structure
The Helm chart SHALL be located at `deploy/helm/secretli/` with Chart.yaml, values.yaml, values-production.yaml, and a templates directory.

#### Scenario: Chart metadata
- **WHEN** `helm lint deploy/helm/secretli/` is run
- **THEN** the chart SHALL pass linting with no errors

### Requirement: Deployment template
The chart SHALL include a Deployment template for the application.

#### Scenario: Deployment creates pods
- **WHEN** the chart is installed
- **THEN** a Deployment SHALL be created with the configured replica count, image, environment variables from ConfigMap and Secret, liveness and readiness probes pointing to health endpoints, and resource limits

### Requirement: Service template
The chart SHALL include a ClusterIP Service.

#### Scenario: Service routes traffic
- **WHEN** the chart is installed
- **THEN** a Service SHALL be created on port 80 targeting container port 8080

### Requirement: Ingress template
The chart SHALL include an optional Ingress resource.

#### Scenario: Ingress enabled
- **WHEN** `ingress.enabled` is true in values
- **THEN** an Ingress resource SHALL be created with the configured hostname, TLS settings, and annotations

#### Scenario: Ingress disabled
- **WHEN** `ingress.enabled` is false (default)
- **THEN** no Ingress resource SHALL be created

### Requirement: ConfigMap for non-sensitive config
The chart SHALL include a ConfigMap for non-sensitive environment variables.

#### Scenario: ConfigMap contains app config
- **WHEN** the chart is installed
- **THEN** a ConfigMap SHALL contain SERVER_PORT, S3_BUCKET, S3_REGION, S3_USE_SSL, CLEANUP_INTERVAL, SESSION_MAX_AGE, COOKIE_SECURE, ALLOWED_ORIGINS, and MAX_FILE_SIZE

### Requirement: Secret for sensitive config
The chart SHALL include a Secret for sensitive environment variables.

#### Scenario: Secret contains credentials
- **WHEN** the chart is installed
- **THEN** a Secret SHALL contain DATABASE_URL, S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, and COOKIE_DOMAIN

### Requirement: Pre-install/upgrade migration job
The chart SHALL include a Job that runs database migrations as a Helm hook.

#### Scenario: Migration runs before install
- **WHEN** `helm install` is run
- **THEN** a Job with `helm.sh/hook: pre-install,pre-upgrade` SHALL run the binary with `migrate` argument before the Deployment starts

#### Scenario: Failed job cleanup
- **WHEN** a previous migration Job exists
- **THEN** the hook-delete-policy `before-hook-creation` SHALL delete it before creating a new one

### Requirement: PodDisruptionBudget
The chart SHALL include a PDB to ensure availability during rolling updates.

#### Scenario: PDB with minAvailable
- **WHEN** the chart is installed with replicas > 1
- **THEN** a PodDisruptionBudget SHALL be created with `minAvailable: 1`

### Requirement: HorizontalPodAutoscaler
The chart SHALL include an optional HPA.

#### Scenario: HPA enabled
- **WHEN** `autoscaling.enabled` is true in values
- **THEN** an HPA SHALL be created with configured min/max replicas and CPU target

#### Scenario: HPA disabled
- **WHEN** `autoscaling.enabled` is false (default)
- **THEN** no HPA resource SHALL be created

### Requirement: ServiceAccount
The chart SHALL include a ServiceAccount.

#### Scenario: ServiceAccount created
- **WHEN** the chart is installed
- **THEN** a ServiceAccount SHALL be created and referenced by the Deployment

### Requirement: Production values override
A `values-production.yaml` SHALL provide production-appropriate defaults.

#### Scenario: Production values differ from defaults
- **WHEN** deploying with `values-production.yaml`
- **THEN** replicas SHALL be >= 2, resource limits SHALL be set, and ingress SHALL be enabled with TLS
