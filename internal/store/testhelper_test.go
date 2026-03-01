package store_test

import (
	"context"
	"embed"
	"io/fs"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pscheid92/secretli/internal/store"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed testdata/migrations/*.sql
var testdataFS embed.FS

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping integration test: docker not found in PATH")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("skipping integration test: docker not available: %v", err)
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("secretli_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			).WithDeadline(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	// fs.Sub strips the "testdata" prefix so RunMigrations sees "migrations/*.sql"
	migrationsFS, err := fs.Sub(testdataFS, "testdata")
	if err != nil {
		t.Fatalf("create sub filesystem: %v", err)
	}

	if err := store.RunMigrations(ctx, connStr, migrationsFS); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}
