package postgres_test

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgadapter "github.com/pscheid92/secretli/internal/adapter/postgres"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	testDBDockerOnce sync.Once
	testDBDockerErr  error

	testDBOnce      sync.Once
	testDBErr       error
	testDBMu        sync.Mutex
	testDBPool      *pgxpool.Pool
	testDBContainer *tcpostgres.PostgresContainer
)

func TestMain(m *testing.M) {
	code := m.Run()
	if testDBPool != nil {
		testDBPool.Close()
	}
	if testDBContainer != nil {
		_ = testDBContainer.Terminate(context.Background())
	}
	os.Exit(code)
}

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	requireDocker(t)
	testDBMu.Lock()
	t.Cleanup(testDBMu.Unlock)

	testDBOnce.Do(startTestDB)
	if testDBErr != nil {
		t.Fatalf("setup postgres test database: %v", testDBErr)
	}

	resetTestDB(t, testDBPool)
	return testDBPool
}

func requireDocker(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testDBDockerOnce.Do(func() {
		if _, err := exec.LookPath("docker"); err != nil {
			testDBDockerErr = err
			return
		}
		testDBDockerErr = exec.Command("docker", "info").Run()
	})
	if testDBDockerErr != nil {
		t.Skipf("skipping integration test: docker unavailable: %v", testDBDockerErr)
	}
}

func startTestDB() {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("secretli_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			).WithDeadline(30*time.Second),
		),
	)
	if err != nil {
		testDBErr = err
		return
	}
	testDBContainer = pgContainer

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		testDBErr = err
		return
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		testDBErr = err
		return
	}
	testDBPool = pool

	if err := pgadapter.RunMigrations(ctx, pool); err != nil {
		testDBErr = err
	}
}

func resetTestDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE upload_parts, upload_sessions, retrieval_sessions, secrets RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("reset postgres test database: %v", err)
	}
}
