package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

// TestMigration004_BackfillsAndScrubs migrates a database holding rows written
// before per-upload storage keys and checks they are carried over.
func TestMigration004_BackfillsAndScrubs(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "DROP DATABASE IF EXISTS secretli_migration_test"); err != nil {
		t.Fatalf("drop database: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE DATABASE secretli_migration_test"); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP DATABASE IF EXISTS secretli_migration_test WITH (FORCE)")
	})

	connString := strings.Replace(pool.Config().ConnString(), "/secretli_test", "/secretli_migration_test", 1)
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	migrator, err := migrate.NewMigrator(ctx, conn, "schema_version")
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	if err := migrator.LoadMigrations(os.DirFS("migrations")); err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if err := migrator.MigrateTo(ctx, 3); err != nil {
		t.Fatalf("migrate to 3: %v", err)
	}

	now := time.Now()
	if _, err := conn.Exec(ctx, `
		INSERT INTO secrets (public_id, metadata_token_hash, blob_token_hash, deletion_token_hash, encrypted_meta, blob_size, expires_at, created_at)
		VALUES ('legacy-secret', 'm', 'b', 'd', 'meta', 1, $1, $2)`, now.Add(time.Hour), now); err != nil {
		t.Fatalf("insert secret: %v", err)
	}
	for _, s := range []struct{ id, state string }{{"legacy-pending", "pending"}, {"legacy-done", "completed"}} {
		if _, err := conn.Exec(ctx, `
			INSERT INTO upload_sessions (session_id, public_id, upload_token_hash, metadata_token_hash, blob_token_hash, deletion_token_hash,
				s3_upload_id, blob_size, encrypted_meta, secret_expires_at, upload_expires_at, state, created_at)
			VALUES ($1, $1 || '-public', 'u', 'm', 'b', 'd', 's3', 1, 'meta', $2, $2, $3, $4)`,
			s.id, now.Add(time.Hour), s.state, now); err != nil {
			t.Fatalf("insert session %s: %v", s.id, err)
		}
		if _, err := conn.Exec(ctx, `
			INSERT INTO upload_parts (session_id, part_number, part_offset, part_size, part_sha256, etag, created_at)
			VALUES ($1, 1, 0, 1, 'sha', 'etag', $2)`, s.id, now); err != nil {
			t.Fatalf("insert part for %s: %v", s.id, err)
		}
	}

	if err := migrator.MigrateTo(ctx, 4); err != nil {
		t.Fatalf("migrate to 4: %v", err)
	}

	var secretKey string
	if err := conn.QueryRow(ctx, "SELECT storage_key FROM secrets WHERE public_id = 'legacy-secret'").Scan(&secretKey); err != nil {
		t.Fatalf("query secret: %v", err)
	}
	if secretKey != "secrets/legacy-secret" {
		t.Errorf("secret storage key = %q, want the legacy key", secretKey)
	}

	for _, tc := range []struct {
		id            string
		wantKey       string
		wantMaterial  int
		wantPartCount int
	}{
		{id: "legacy-pending", wantKey: "secrets/legacy-pending-public", wantMaterial: 4, wantPartCount: 1},
		{id: "legacy-done", wantKey: "secrets/legacy-done-public", wantMaterial: 0, wantPartCount: 0},
	} {
		var key string
		var material, parts int
		if err := conn.QueryRow(ctx, `
			SELECT storage_key,
			       num_nonnulls(metadata_token_hash, blob_token_hash, deletion_token_hash, encrypted_meta),
			       (SELECT count(*) FROM upload_parts p WHERE p.session_id = s.session_id)
			FROM upload_sessions s WHERE session_id = $1`, tc.id).Scan(&key, &material, &parts); err != nil {
			t.Fatalf("query %s: %v", tc.id, err)
		}
		if key != tc.wantKey || material != tc.wantMaterial || parts != tc.wantPartCount {
			t.Errorf("%s: key = %q, share columns = %d, parts = %d; want %q, %d, %d",
				tc.id, key, material, parts, tc.wantKey, tc.wantMaterial, tc.wantPartCount)
		}
	}

	// The migration can be rolled back.
	if err := migrator.MigrateTo(ctx, 3); err != nil {
		t.Fatalf("migrate down to 3: %v", err)
	}
}
