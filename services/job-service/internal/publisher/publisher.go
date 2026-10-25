// services/job-service/internal/publisher/publisher.go

// Package publisher defines the Publisher interface and its RabbitMQ implementation
// used by the outbox relay to forward job events to a message broker.
package publisher

import "context"

// Publisher abstracts message-broker publishing so the outbox relay is not
// coupled to any specific broker implementation.
type Publisher interface {
	Publish(ctx context.Context, body []byte) error
	Close() error
}
