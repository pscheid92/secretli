## 1. PostgreSQL Upgrade

- [x] 1.1 Update `docker/docker-compose.yml` — change `postgres:17-alpine` to `postgres:18-alpine`
- [x] 1.2 Update `docker/docker-compose.dev.yml` — change `postgres:17-alpine` to `postgres:18-alpine`

## 2. Database Migration

- [x] 2.1 Create `migrations/006_uuidv7_secrets_id.sql` — drop `BIGSERIAL id` column, add `UUID id` with `DEFAULT uuidv7()` as primary key

## 3. sqlc Configuration and Regeneration

- [x] 3.1 Add type override in `sqlc.yaml` mapping PostgreSQL `uuid` to `github.com/google/uuid.UUID`
- [x] 3.2 Run `sqlc generate` to regenerate `internal/store/dbsqlc/` with `uuid.UUID` types

## 4. Go Model and Store Layer

- [x] 4.1 Update `internal/model/secret.go` — change `ID int64` to `ID uuid.UUID`, add `github.com/google/uuid` import
- [x] 4.2 Update `internal/store/secret_repo_pg.go` — ensure `secretFromRow` and `Create` handle `uuid.UUID` correctly
- [x] 4.3 Update `internal/store/secret_repo_test.go` — change `got.ID == 0` assertion to `got.ID == uuid.Nil`

## 5. Dependency Cleanup

- [x] 5.1 Run `go mod tidy` to promote `github.com/google/uuid` to direct dependency

## 6. Verification

- [x] 6.1 Run `go build ./...` to confirm compilation
- [x] 6.2 Run integration tests against PostgreSQL 18 to confirm all pass
