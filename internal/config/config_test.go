package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.S3Bucket != "secretli" {
		t.Errorf("S3Bucket = %q, want %q", cfg.S3Bucket, "secretli")
	}
	if cfg.S3UseSSL != true {
		t.Errorf("S3UseSSL = %v, want true", cfg.S3UseSSL)
	}
	if cfg.S3Region != "us-east-1" {
		t.Errorf("S3Region = %q, want %q", cfg.S3Region, "us-east-1")
	}
	if cfg.MaxFileSize != 104857600 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 104857600)
	}
	if cfg.CleanupInterval != time.Minute {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, time.Minute)
	}
	if cfg.SessionMaxAge != 720*time.Hour {
		t.Errorf("SessionMaxAge = %v, want %v", cfg.SessionMaxAge, 720*time.Hour)
	}
	if cfg.CookieSecure != true {
		t.Errorf("CookieSecure = %v, want true", cfg.CookieSecure)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("S3_BUCKET", "my-bucket")
	t.Setenv("S3_USE_SSL", "false")
	t.Setenv("MAX_FILE_SIZE", "5242880")
	t.Setenv("CLEANUP_INTERVAL", "5m")
	t.Setenv("SESSION_MAX_AGE", "48h")
	t.Setenv("COOKIE_SECURE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.S3Bucket != "my-bucket" {
		t.Errorf("S3Bucket = %q, want %q", cfg.S3Bucket, "my-bucket")
	}
	if cfg.S3UseSSL != false {
		t.Errorf("S3UseSSL = %v, want false", cfg.S3UseSSL)
	}
	if cfg.MaxFileSize != 5242880 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 5242880)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, 5*time.Minute)
	}
	if cfg.SessionMaxAge != 48*time.Hour {
		t.Errorf("SessionMaxAge = %v, want %v", cfg.SessionMaxAge, 48*time.Hour)
	}
	if cfg.CookieSecure != false {
		t.Errorf("CookieSecure = %v, want false", cfg.CookieSecure)
	}
}

func TestLoadInvalidEnvReturnsError(t *testing.T) {
	t.Setenv("S3_USE_SSL", "notabool")

	_, err := Load()
	if err == nil {
		t.Error("Load() should return error for invalid bool value")
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := "# comment\nSERVER_PORT=3000\nS3_BUCKET=test-bucket\n"
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Ensure vars are unset before calling loadDotEnv.
	// t.Setenv snapshots the original value and restores it on cleanup.
	t.Setenv("SERVER_PORT", "")
	os.Unsetenv("SERVER_PORT")
	t.Setenv("S3_BUCKET", "")
	os.Unsetenv("S3_BUCKET")

	loadDotEnv(envFile)

	if v := os.Getenv("SERVER_PORT"); v != "3000" {
		t.Errorf("SERVER_PORT = %q, want %q", v, "3000")
	}
	if v := os.Getenv("S3_BUCKET"); v != "test-bucket" {
		t.Errorf("S3_BUCKET = %q, want %q", v, "test-bucket")
	}
}

func TestLoadDotEnvRealEnvTakesPrecedence(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")

	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("SERVER_PORT=3000\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loadDotEnv(envFile)

	if v := os.Getenv("SERVER_PORT"); v != "9090" {
		t.Errorf("SERVER_PORT = %q, want %q (real env should take precedence)", v, "9090")
	}
}

func TestLoadDotEnvQuotedValues(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := "COOKIE_DOMAIN=\"example.com\"\nALLOWED_ORIGINS='http://localhost:3000'\n"
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("COOKIE_DOMAIN", "")
	os.Unsetenv("COOKIE_DOMAIN")
	t.Setenv("ALLOWED_ORIGINS", "")
	os.Unsetenv("ALLOWED_ORIGINS")

	loadDotEnv(envFile)

	if v := os.Getenv("COOKIE_DOMAIN"); v != "example.com" {
		t.Errorf("COOKIE_DOMAIN = %q, want %q", v, "example.com")
	}
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "http://localhost:3000" {
		t.Errorf("ALLOWED_ORIGINS = %q, want %q", v, "http://localhost:3000")
	}
}
