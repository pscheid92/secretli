## Why

The store layer has ~400 lines of hand-written SQL with manual `rows.Scan()` calls and positional parameter binding across 4 repo implementations. This is error-prone — the `secret.ID` bug (using `Exec` instead of `QueryRow` with `RETURNING id`) is exactly the class of bug sqlc prevents by generating type-safe Go code from SQL at compile time.

## What Changes

- Add sqlc as a dev dependency and configure it for pgx v5
- Extract all inline SQL queries from `*_repo_pg.go` files into `.sql` query files
- Replace the 4 hand-written Postgres repo implementations with sqlc-generated code
- Keep the existing repo interfaces unchanged — handlers continue to use `store.SecretRepo`, `store.UserRepo`, etc.
- Add thin adapter layers where sqlc's generated types differ from `model.*` types
- Keep custom error handling (`ErrNotFound`, `ErrDuplicate`, `ErrDuplicateEmail`) by wrapping sqlc's generated functions

## Capabilities

### New Capabilities

_(none — this is a refactor, not a new feature)_

### Modified Capabilities

_(none — repo interfaces and behavior are unchanged; only the implementation changes from hand-written to generated code)_

## Impact

- **Code**: `internal/store/` — `secret_repo_pg.go`, `user_repo_pg.go`, `session_repo_pg.go`, `user_secret_repo_pg.go` replaced with sqlc-generated code + thin wrappers
- **New files**: `sqlc.yaml` config, `internal/store/queries/*.sql` query files, `internal/store/sqlc/` generated package
- **Dependencies**: Add `github.com/sqlc-dev/sqlc` as a dev/build tool (not a Go module dependency — sqlc generates code, it doesn't import at runtime)
- **Tests**: Existing integration tests in `internal/store/*_test.go` remain unchanged — they test via the repo interfaces
- **Models**: `model.Secret`, `model.User` unchanged. sqlc generates its own row types internally; adapters map between them
