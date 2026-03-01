## 1. Setup

- [x] 1.1 Install sqlc CLI and add `sqlc.yaml` config pointing at `migrations/` for schema and `internal/store/queries/` for queries, outputting to `internal/store/dbsqlc/`
- [x] 1.2 Add `go generate` directive for sqlc in the store package

## 2. Extract SQL queries

- [x] 2.1 Create `internal/store/queries/secrets.sql` with all 6 SecretRepo queries (Create, GetByPublicID, GetAndDeleteByPublicID, SetRetrievedAt, Delete, DeleteExpired)
- [x] 2.2 Create `internal/store/queries/users.sql` with all 3 UserRepo queries (Create, GetByEmail, GetByID)
- [x] 2.3 Create `internal/store/queries/sessions.sql` with all 4 SessionRepo queries (Create, GetByIDWithUser, Delete, DeleteExpiredSessions)
- [x] 2.4 Create `internal/store/queries/user_secrets.sql` with all 3 UserSecretRepo queries (LinkSecret, CountByUser, ListByUser)

## 3. Generate and verify

- [x] 3.1 Run `sqlc generate` and verify generated code compiles

## 4. Replace repo implementations

- [x] 4.1 Rewrite `secret_repo_pg.go` as thin wrapper over sqlc-generated functions with `model.Secret` mapping and error wrapping
- [x] 4.2 Rewrite `user_repo_pg.go` as thin wrapper over sqlc-generated functions with `model.User` mapping and error wrapping
- [x] 4.3 Rewrite `session_repo_pg.go` as thin wrapper over sqlc-generated functions (keep session ID generation in Go)
- [x] 4.4 Rewrite `user_secret_repo_pg.go` as thin wrapper over sqlc-generated functions with `SecretSummary` mapping

## 5. Verification

- [x] 5.1 `go build ./...` passes
- [x] 5.2 `go test ./internal/store/...` passes (existing integration tests)
- [x] 5.3 `go test ./internal/handler/...` passes
- [x] 5.4 `go test ./internal/...` passes (all internal packages)
