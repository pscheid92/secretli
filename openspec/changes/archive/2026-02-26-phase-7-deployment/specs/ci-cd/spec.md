## ADDED Requirements

### Requirement: CI workflow runs on push and PR
The GitHub Actions workflow SHALL trigger on pushes to main and on pull requests.

#### Scenario: PR triggers CI
- **WHEN** a pull request is opened or updated
- **THEN** the lint, test, and build jobs SHALL run

#### Scenario: Main push triggers CI and image push
- **WHEN** a commit is pushed to main
- **THEN** the lint, test, build, and Docker image push jobs SHALL run

### Requirement: Lint job
The CI SHALL run linting for both Go and frontend code.

#### Scenario: Go linting
- **WHEN** the lint job runs
- **THEN** it SHALL run `go vet ./...`

#### Scenario: Frontend linting
- **WHEN** the lint job runs
- **THEN** it SHALL run the frontend ESLint check

### Requirement: Test job
The CI SHALL run tests for both Go and frontend code.

#### Scenario: Go tests
- **WHEN** the test job runs
- **THEN** it SHALL run `go test ./...`

#### Scenario: Frontend tests
- **WHEN** the test job runs
- **THEN** it SHALL run the frontend test suite via npm

### Requirement: Docker build and push
The CI SHALL build and push the Docker image on main branch.

#### Scenario: Image built on main
- **WHEN** CI runs on the main branch
- **THEN** the Docker image SHALL be built and pushed to the configured container registry

#### Scenario: Image not pushed on PR
- **WHEN** CI runs on a pull request
- **THEN** the Docker image SHALL be built (for validation) but NOT pushed
