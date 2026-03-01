package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Advisory lock ID for migrations (arbitrary fixed constant).
const migrationLockID = 0x5365637265746C69 // "Secretli" in hex

func RunMigrations(ctx context.Context, dbURL string, migrationsFS fs.FS) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Acquire a session-level advisory lock so concurrent instances don't race.
	slog.Info("acquiring migration lock")
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID); err != nil {
			slog.Error("failed to release migration lock", "error", err)
		}
	}()

	// Find the migrations subdirectory in the embedded FS
	dirEntries, _ := fs.ReadDir(migrationsFS, "migrations")
	dir := "migrations"
	if len(dirEntries) == 0 {
		dir = "."
	}

	goose.SetBaseFS(migrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}
