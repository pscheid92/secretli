package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func RunMigrations(ctx context.Context, dbURL string, migrationsFS fs.FS) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

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
