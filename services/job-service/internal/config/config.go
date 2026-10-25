// services/job-service/internal/config/config.go

// Package config loads and validates job-service runtime configuration
// from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the runtime configuration loaded from environment variables.
type Config struct {
	Port               string
	DatabaseURL        string
	RabbitMQURL        string
	// OutboxPollInterval controls how often the outbox relay goroutine queries
	// for unpublished events; defaults to 500 ms when OUTBOX_POLL_MS is absent or invalid.
	OutboxPollInterval time.Duration
}

// Load reads configuration from the environment and returns an error if a required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		Port: getEnv("JOB_SERVICE_PORT", "8081"),
	}
	var err error
	if cfg.DatabaseURL, err = requireEnv("DATABASE_URL"); err != nil {
		return nil, err
	}
	if cfg.RabbitMQURL, err = requireEnv("RABBITMQ_URL"); err != nil {
		return nil, err
	}
	// defaultPollMS is the fallback poll interval in milliseconds used when OUTBOX_POLL_MS is absent or invalid.
	const defaultPollMS = 500
	ms, err := strconv.ParseInt(getEnv("OUTBOX_POLL_MS", "500"), 10, 64)
	if err != nil || ms <= 0 {
		ms = defaultPollMS
	}
	cfg.OutboxPollInterval = time.Duration(ms) * time.Millisecond
	return cfg, nil
}

// getEnv returns the value of the named environment variable,
// falling back to fallback when the variable is empty or unset.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requireEnv returns the value of the named environment variable or an error when the variable is not set.
func requireEnv(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("required environment variable %q is not set", key)
}
