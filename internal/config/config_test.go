package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load()

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

	cfg := Load()

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

func TestLoadInvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("S3_USE_SSL", "notabool")
	t.Setenv("MAX_FILE_SIZE", "notanumber")
	t.Setenv("CLEANUP_INTERVAL", "notaduration")

	cfg := Load()

	if cfg.S3UseSSL != true {
		t.Errorf("S3UseSSL = %v, want true (default)", cfg.S3UseSSL)
	}
	if cfg.MaxFileSize != 104857600 {
		t.Errorf("MaxFileSize = %d, want %d (default)", cfg.MaxFileSize, 104857600)
	}
	if cfg.CleanupInterval != time.Minute {
		t.Errorf("CleanupInterval = %v, want %v (default)", cfg.CleanupInterval, time.Minute)
	}
}
