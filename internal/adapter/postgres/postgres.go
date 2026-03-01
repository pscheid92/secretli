//go:generate sqlc generate -f ../../../sqlc.yaml

package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/tern/v2/migrate"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Advisory lock ID for migrations (arbitrary fixed constant).
const migrationLockID = 0x5365637265746C69 // "Secretli" in hex

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migrations: %w", err)
	}
	defer conn.Release()

	slog.Info("acquiring migration lock")
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID); err != nil {
			slog.Error("failed to release migration lock", "error", err)
		}
	}()

	migrator, err := migrate.NewMigrator(ctx, conn.Conn(), "schema_version")
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	migrationFiles, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migrations sub-FS: %w", err)
	}

	if err := migrator.LoadMigrations(migrationFiles); err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
