// services/job-service/internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port               string
	DatabaseURL        string
	RabbitMQURL        string
	OutboxPollInterval time.Duration
}

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
	ms, _ := strconv.ParseInt(getEnv("OUTBOX_POLL_MS", "500"), 10, 64)
	cfg.OutboxPollInterval = time.Duration(ms) * time.Millisecond
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
