package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"image-platform/job-service/internal/config"
)

// setRequiredEnv sets the minimum required env vars for Load() to succeed.
// t.Setenv restores the original value when the test ends.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/testdb")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost/")
}

func TestOutboxPollInterval_Default(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OUTBOX_POLL_MS", "") // unset

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, cfg.OutboxPollInterval,
		"unset OUTBOX_POLL_MS should default to 500ms")
}

func TestOutboxPollInterval_Zero(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OUTBOX_POLL_MS", "0")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, cfg.OutboxPollInterval,
		"OUTBOX_POLL_MS=0 should fall back to 500ms (guard against ticker panic)")
}

func TestOutboxPollInterval_NonNumeric(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OUTBOX_POLL_MS", "abc")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 500*time.Millisecond, cfg.OutboxPollInterval,
		"non-numeric OUTBOX_POLL_MS should fall back to 500ms")
}

func TestOutboxPollInterval_ValidOverride(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OUTBOX_POLL_MS", "250")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 250*time.Millisecond, cfg.OutboxPollInterval,
		"valid OUTBOX_POLL_MS=250 should be respected")
}
