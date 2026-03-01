## MODIFIED Requirements

### Requirement: Docker Compose provides full local stack
The docker-compose.yml SHALL include an `app` service alongside postgres and minio for full local stack testing.

#### Scenario: App service builds from Dockerfile
- **WHEN** `docker compose up` is run
- **THEN** the app service SHALL build from the project Dockerfile and start the HTTP server on port 8080

#### Scenario: App connects to local services
- **WHEN** the app service starts
- **THEN** it SHALL connect to the postgres and minio services via Docker networking using appropriate environment variables

#### Scenario: App depends on infrastructure services
- **WHEN** docker compose starts services
- **THEN** the app service SHALL wait for postgres and minio to be ready before starting
