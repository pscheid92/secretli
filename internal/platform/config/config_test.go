package config

import (
	"testing"
	"time"
)

// setRequiredEnv sets the required env vars so Load() doesn't fail.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("S3_ENDPOINT", "localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "minioadmin")
	t.Setenv("S3_SECRET_KEY", "minioadmin")
}

func TestLoadDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.S3.Bucket != "secretli" {
		t.Errorf("S3Bucket = %q, want %q", cfg.S3.Bucket, "secretli")
	}
	if cfg.S3.UseSSL != true {
		t.Errorf("S3UseSSL = %v, want true", cfg.S3.UseSSL)
	}
	if cfg.S3.Region != "us-east-1" {
		t.Errorf("S3Region = %q, want %q", cfg.S3.Region, "us-east-1")
	}
	if cfg.MaxFileSize != 104857600 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 104857600)
	}
	if cfg.CleanupInterval != time.Minute {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, time.Minute)
	}
	if cfg.MetricsToken != "" {
		t.Errorf("MetricsToken = %q, want empty", cfg.MetricsToken)
	}
}

func TestLoadFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("S3_BUCKET", "my-bucket")
	t.Setenv("S3_USE_SSL", "false")
	t.Setenv("MAX_FILE_SIZE", "5242880")
	t.Setenv("CLEANUP_INTERVAL", "5m")
	t.Setenv("METRICS_TOKEN", "metrics-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.S3.Bucket != "my-bucket" {
		t.Errorf("S3Bucket = %q, want %q", cfg.S3.Bucket, "my-bucket")
	}
	if cfg.S3.UseSSL != false {
		t.Errorf("S3UseSSL = %v, want false", cfg.S3.UseSSL)
	}
	if cfg.MaxFileSize != 5242880 {
		t.Errorf("MaxFileSize = %d, want %d", cfg.MaxFileSize, 5242880)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("CleanupInterval = %v, want %v", cfg.CleanupInterval, 5*time.Minute)
	}
	if cfg.MetricsToken != "metrics-secret" {
		t.Errorf("MetricsToken = %q, want %q", cfg.MetricsToken, "metrics-secret")
	}
}

func TestLoadMissingRequiredReturnsError(t *testing.T) {
	// No required env vars set — should fail
	_, err := Load()
	if err == nil {
		t.Error("Load() should return error when required env vars are missing")
	}
}

func TestLoadInvalidEnvReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("S3_USE_SSL", "notabool")

	_, err := Load()
	if err == nil {
		t.Error("Load() should return error for invalid bool value")
	}
}
