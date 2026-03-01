package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	S3Endpoint      string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	S3UseSSL        bool
	S3Region        string
	MaxFileSize     int64
	CleanupInterval time.Duration
	SessionMaxAge   time.Duration
	CookieDomain    string
	CookieSecure    bool
	AllowedOrigins  string
}

func Load() Config {
	return Config{
		Port:            envOrDefault("SERVER_PORT", "8080"),
		DatabaseURL:     envOrDefault("DATABASE_URL", ""),
		S3Endpoint:      envOrDefault("S3_ENDPOINT", ""),
		S3Bucket:        envOrDefault("S3_BUCKET", "secretli"),
		S3AccessKey:     envOrDefault("S3_ACCESS_KEY", ""),
		S3SecretKey:     envOrDefault("S3_SECRET_KEY", ""),
		S3UseSSL:        envOrDefaultBool("S3_USE_SSL", true),
		S3Region:        envOrDefault("S3_REGION", "us-east-1"),
		MaxFileSize:     envOrDefaultInt64("MAX_FILE_SIZE", 104857600),
		CleanupInterval: envOrDefaultDuration("CLEANUP_INTERVAL", time.Minute),
		SessionMaxAge:   envOrDefaultDuration("SESSION_MAX_AGE", 720*time.Hour),
		CookieDomain:    envOrDefault("COOKIE_DOMAIN", ""),
		CookieSecure:    envOrDefaultBool("COOKIE_SECURE", true),
		AllowedOrigins:  envOrDefault("ALLOWED_ORIGINS", ""),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envOrDefaultInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envOrDefaultDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
