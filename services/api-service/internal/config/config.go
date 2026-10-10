package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             string
	DatabaseURL      string
	MinioEndpoint    string
	MinioAccessKey   string
	MinioSecretKey   string
	MinioUseSSL      bool
	MaxUploadBytes   int64
	MaxMegapixels    int64
	PresignedURLTTL  time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Port: getEnv("API_PORT", "8080"),
	}

	var err error
	if cfg.DatabaseURL, err = requireEnv("DATABASE_URL"); err != nil {
		return nil, err
	}
	if cfg.MinioEndpoint, err = requireEnv("MINIO_ENDPOINT"); err != nil {
		return nil, err
	}
	if cfg.MinioAccessKey, err = requireEnv("MINIO_ACCESS_KEY"); err != nil {
		return nil, err
	}
	if cfg.MinioSecretKey, err = requireEnv("MINIO_SECRET_KEY"); err != nil {
		return nil, err
	}

	ssl := getEnv("MINIO_USE_SSL", "false")
	cfg.MinioUseSSL = ssl == "true" || ssl == "1"

	maxMB, _ := strconv.ParseInt(getEnv("MAX_UPLOAD_SIZE_MB", "10"), 10, 64)
	cfg.MaxUploadBytes = maxMB * 1024 * 1024

	cfg.MaxMegapixels, _ = strconv.ParseInt(getEnv("MAX_MEGAPIXELS", "50000000"), 10, 64)

	ttlMin, _ := strconv.ParseInt(getEnv("PRESIGNED_URL_TTL_MINUTES", "60"), 10, 64)
	cfg.PresignedURLTTL = time.Duration(ttlMin) * time.Minute

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("required environment variable %q is not set", key)
}
