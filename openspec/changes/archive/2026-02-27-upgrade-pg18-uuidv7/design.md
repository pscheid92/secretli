## Context

The project runs PostgreSQL 17 with `BIGSERIAL` primary keys on the `secrets` table. All lookups use the client-derived `public_id` (TEXT) — the internal `id` is never exposed in APIs or URLs. PostgreSQL 18 (released 2025) adds the built-in `uuidv7()` function, removing the need for extensions like `pgcrypto` or `uuid-ossp` to generate UUIDs.

## Goals / Non-Goals

**Goals:**
- Upgrade PostgreSQL from 17 to 18 in all Docker configurations
- Replace `BIGSERIAL` primary key with `UUID DEFAULT uuidv7()` on the `secrets` table
- Update the Go data layer to use `uuid.UUID` instead of `int64` for the ID field

**Non-Goals:**
- Changing the `public_id` column (it's part of the zero-knowledge crypto protocol)
- Adding UUID columns to any other purpose (no new tables or columns)
- Data migration of existing rows (secrets are ephemeral; the migration drops and re-adds the column)

## Decisions

### Use `google/uuid` over `pgtype.UUID` in Go model

The model layer (`model.Secret`) uses standard Go types, not pgx-specific types. Using `google/uuid.UUID` keeps the model clean and provides a richer API (`.String()`, comparison, `uuid.Nil`). sqlc supports this via type overrides. The package is already an indirect dependency.

**Alternative**: Use `pgtype.UUID` — rejected because it leaks database concerns into the domain model.

### Column replacement migration (drop + add) over ALTER TYPE

PostgreSQL cannot `ALTER COLUMN TYPE` from `bigint` to `uuid` directly. Rather than a multi-step add-column/copy/swap approach, we drop the constraint and column, then add a new UUID column with `DEFAULT uuidv7()`. This is safe because:
- Secrets are ephemeral (they expire)
- The `id` is never referenced by foreign keys
- The `id` is never exposed externally

### Keep `RETURNING id` in CreateSecret query

The query still returns the generated `id` after insert. This maintains the pattern of populating `secret.ID` after creation, useful for logging and debugging.

## Risks / Trade-offs

- **[Existing data loss on migration]** → Acceptable: secrets are ephemeral and expire. Any production deployment should schedule the migration during low usage.
- **[PG 18 image availability]** → `postgres:18-alpine` is available on Docker Hub since Sept 2025.
- **[UUID storage overhead]** → UUID is 16 bytes vs 8 bytes for bigint. Negligible for this workload.
