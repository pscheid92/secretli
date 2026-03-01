## Context

The store layer has 4 Postgres repo implementations totaling ~400 lines of hand-written SQL with manual `rows.Scan()` calls. Every query uses positional parameters (`$1`, `$2`, ...) and manual struct-to-column mapping. Adding or reordering columns requires updating both the SQL string and the Scan call in lockstep — a common source of bugs (e.g., the `RETURNING id` bug we fixed earlier).

sqlc is a compile-time tool that generates type-safe Go code from SQL queries. It reads `.sql` query files, validates them against the schema, and generates Go functions with proper parameter and return types. It uses pgx v5 natively.

## Goals / Non-Goals

**Goals:**
- Replace all hand-written `*_repo_pg.go` implementations with sqlc-generated code
- Keep existing repo interfaces (`SecretRepo`, `UserRepo`, `SessionRepo`, `UserSecretRepo`) unchanged
- Keep existing integration tests passing without modification
- Keep custom error handling (`ErrNotFound`, `ErrDuplicate`, `ErrDuplicateEmail`)
- Keep using `model.Secret` and `model.User` as the types handlers work with

**Non-Goals:**
- Changing the repo interfaces or adding new queries
- Changing the database schema or migrations
- Replacing pgxpool connection management
- Migrating the migration runner (goose stays)

## Decisions

### 1. sqlc output goes to `internal/store/sqlc/`

**Choice:** Generate sqlc code into a sub-package `internal/store/sqlc/` with its own types.

**Rationale:** Keeps generated code separate from hand-written wrappers. The `internal/store/` package continues to own the repo interfaces and error types. The `sqlc/` sub-package is an implementation detail.

**Alternative considered:** Generate directly into `internal/store/`. Rejected because it would mix generated and hand-written code, making `go generate` messy and diffs hard to review.

### 2. Query files organized by entity

**Choice:** One `.sql` file per entity: `secrets.sql`, `users.sql`, `sessions.sql`, `user_secrets.sql`.

**Rationale:** Maps cleanly to the existing repo split. Easy to find queries.

### 3. Thin wrapper repos implement existing interfaces

**Choice:** Each `*_repo_pg.go` becomes a thin wrapper that calls sqlc-generated functions and maps between `sqlc.*Row` types and `model.*` types, plus error wrapping.

**Rationale:** sqlc generates its own row structs (e.g., `sqlc.Secret`) which won't match our `model.Secret` exactly (different JSON tags, different field visibility). The wrapper handles this mapping and preserves the `ErrNotFound`/`ErrDuplicate` error semantics.

### 4. Use sqlc's pgx/v5 engine

**Choice:** Configure sqlc with `engine: postgresql` and `sql_package: pgx/v5`.

**Rationale:** We already use pgx/v5 throughout. sqlc supports it natively with `pgx.Rows` scanning.

### 5. Schema provided via migration files

**Choice:** Point sqlc's `schema` config at the `migrations/` directory so it reads the table definitions from the goose migration files.

**Rationale:** Single source of truth for schema. No need to maintain a separate `schema.sql`.

### 6. Session ID generation stays in Go

**Choice:** Keep the random session ID generation (`crypto/rand` + hex encoding) in the wrapper, not in SQL.

**Rationale:** sqlc generates functions from SQL queries. The session Create involves Go-side random generation before the INSERT. The wrapper handles this, then calls the sqlc-generated insert.

## Risks / Trade-offs

- **[sqlc as build dependency]** → sqlc is a CLI tool, not a Go import. Add it to the project via `go install` or as a binary in CI. Developers need it installed to regenerate, but the generated code is committed so it's not needed for regular builds.
- **[Type mapping boilerplate]** → Each wrapper needs a mapping function between `sqlc.Row` and `model.*`. This is ~5-10 lines per entity — less code than the current manual Scan calls, and type-checked at compile time.
- **[Query changes require regeneration]** → Any SQL change requires running `sqlc generate`. This is a `go generate` step. Risk is low since it's a standard Go workflow.
