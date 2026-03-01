package config

import (
	"bufio"
	"os"
	"strings"
	"time"

	"go-simpler.org/env"
)

type Config struct {
	Port            string        `env:"SERVER_PORT" default:"8080"`
	DatabaseURL     string        `env:"DATABASE_URL"`
	S3Endpoint      string        `env:"S3_ENDPOINT"`
	S3Bucket        string        `env:"S3_BUCKET" default:"secretli"`
	S3AccessKey     string        `env:"S3_ACCESS_KEY"`
	S3SecretKey     string        `env:"S3_SECRET_KEY"`
	S3UseSSL        bool          `env:"S3_USE_SSL" default:"true"`
	S3Region        string        `env:"S3_REGION" default:"us-east-1"`
	MaxFileSize     int64         `env:"MAX_FILE_SIZE" default:"104857600"`
	CleanupInterval time.Duration `env:"CLEANUP_INTERVAL" default:"1m"`
	SessionMaxAge   time.Duration `env:"SESSION_MAX_AGE" default:"720h"`
	CookieDomain    string        `env:"COOKIE_DOMAIN"`
	CookieSecure    bool          `env:"COOKIE_SECURE" default:"true"`
	AllowedOrigins  string        `env:"ALLOWED_ORIGINS"`
}

func Load() (Config, error) {
	loadDotEnv(".env")
	var cfg Config
	if err := env.Load(&cfg, nil); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}
