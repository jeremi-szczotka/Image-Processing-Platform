// services/job-service/internal/outbox/relay.go

// Package outbox implements the transactional-outbox relay that polls PostgreSQL
// for unpublished events and forwards them to RabbitMQ, providing at-least-once delivery.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"image-platform/job-service/internal/publisher"
	"image-platform/job-service/internal/repository"
)

// Relay polls the outbox_events table and publishes pending events to the message broker.
// It implements at-least-once delivery: if the broker acknowledges a publish but the
// subsequent MarkPublished call fails, the event is re-published on the next tick.
type Relay struct {
	outbox    repository.OutboxRepository
	publisher publisher.Publisher
	// interval is how often the relay polls for unpublished outbox events.
	interval  time.Duration
	log       *slog.Logger
}

// NewRelay creates a Relay with the supplied outbox repository, publisher, poll interval, and logger.
func NewRelay(
	outbox repository.OutboxRepository,
	pub publisher.Publisher,
	interval time.Duration,
	log *slog.Logger,
) *Relay {
	return &Relay{outbox: outbox, publisher: pub, interval: interval, log: log}
}

// Run polls the outbox at every interval until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.PublishBatch(ctx)
		}
	}
}

// PublishBatch fetches unpublished events and publishes each one.
// If publish succeeds but MarkPublished fails, the event will be re-published on the next tick (at-least-once delivery).
func (r *Relay) PublishBatch(ctx context.Context) {
	events, err := r.outbox.FetchUnpublished(ctx, 100)
	if err != nil {
		r.log.Error("fetch unpublished outbox events", "err", err)
		return
	}
	for _, ev := range events {
		if err := r.publisher.Publish(ctx, ev.Payload); err != nil {
			r.log.Error("publish outbox event", "err", err, "eventID", ev.ID)
			continue
		}
		if err := r.outbox.MarkPublished(ctx, ev.ID); err != nil {
			r.log.Error("mark outbox event published", "err", err, "eventID", ev.ID)
		}
	}
}
