package config

import (
	"time"

	"github.com/joho/godotenv"
	"go-simpler.org/env"
)

type Config struct {
	Port            string        `env:"SERVER_PORT" default:"8080"`
	DatabaseURL     string        `env:"DATABASE_URL,required"`
	S3Endpoint      string        `env:"S3_ENDPOINT,required"`
	S3Bucket        string        `env:"S3_BUCKET" default:"secretli"`
	S3AccessKey     string        `env:"S3_ACCESS_KEY,required"`
	S3SecretKey     string        `env:"S3_SECRET_KEY,required"`
	S3UseSSL        bool          `env:"S3_USE_SSL" default:"true"`
	S3Region        string        `env:"S3_REGION" default:"us-east-1"`
	MaxFileSize     int64         `env:"MAX_FILE_SIZE" default:"104857600"`
	CleanupInterval time.Duration `env:"CLEANUP_INTERVAL" default:"1m"`
	AllowedOrigins  string        `env:"ALLOWED_ORIGINS"`
}

func Load() (Config, error) {
	_ = godotenv.Load() // ignore missing .env
	var cfg Config
	if err := env.Load(&cfg, nil); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
