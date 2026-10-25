// Package config loads api-service runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the runtime configuration loaded from environment variables.
// Required variables (DATABASE_URL, MINIO_*) cause Load to return an error if absent;
// optional variables fall back to safe defaults.
type Config struct {
	Port             string
	DatabaseURL      string
	MinioEndpoint    string
	MinioAccessKey   string
	MinioSecretKey   string
	MinioUseSSL      bool
	// MaxUploadBytes is the maximum accepted request body size in bytes, derived from MAX_UPLOAD_SIZE_MB.
	MaxUploadBytes   int64
	// MaxMegapixels is the maximum total pixel count (width × height) allowed before an upload is rejected
	// to prevent decompression "pixel bomb" attacks. Configured via MAX_MEGAPIXELS.
	MaxMegapixels    int64
	// PresignedURLTTL controls how long presigned download URLs remain valid. Configured via PRESIGNED_URL_TTL_MINUTES.
	PresignedURLTTL  time.Duration
}

// Load reads configuration from the environment and returns a populated Config.
// It returns an error if any required variable is absent.
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

// getEnv returns the value of the environment variable key, or fallback if the variable is unset or empty.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requireEnv returns the value of the environment variable key, or an error if the variable is unset or empty.
func requireEnv(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("required environment variable %q is not set", key)
}
