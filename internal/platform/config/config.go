package config

import (
	"time"

	"github.com/joho/godotenv"
	"go-simpler.org/env"
)

type S3Config struct {
	Endpoint  string `env:"ENDPOINT,required"`
	Bucket    string `env:"BUCKET" default:"secretli"`
	AccessKey string `env:"ACCESS_KEY,required"`
	SecretKey string `env:"SECRET_KEY,required"`
	UseSSL    bool   `env:"USE_SSL" default:"true"`
	Region    string `env:"REGION" default:"us-east-1"`
}

type Config struct {
	Port            string        `env:"SERVER_PORT" default:"8080"`
	DatabaseURL     string        `env:"DATABASE_URL,required"`
	S3              S3Config      `env:"S3"`
	MaxFileSize     int64         `env:"MAX_FILE_SIZE" default:"104857600"`
	CleanupInterval time.Duration `env:"CLEANUP_INTERVAL" default:"1m"`
	AllowedOrigins  string        `env:"ALLOWED_ORIGINS"`
}

func Load() (Config, error) {
	_ = godotenv.Load() // ignore missing .env
	var cfg Config
	if err := env.Load(&cfg, &env.Options{NameSep: "_"}); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
