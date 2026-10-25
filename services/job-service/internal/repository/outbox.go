package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxEvent is a row from the outbox_events table, carrying the JSON payload
// that the outbox relay forwards to RabbitMQ.
type OutboxEvent struct {
	ID      uuid.UUID
	JobID   uuid.UUID
	Payload []byte
}

// OutboxRepository abstracts reads and status updates on the outbox_events table.
// PostgresOutboxRepository is the production implementation.
type OutboxRepository interface {
	FetchUnpublished(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
}

// PostgresOutboxRepository is the PostgreSQL-backed implementation of OutboxRepository.
type PostgresOutboxRepository struct {
	db *pgxpool.Pool
}

// NewPostgresOutboxRepository creates a PostgresOutboxRepository backed by the supplied connection pool.
func NewPostgresOutboxRepository(db *pgxpool.Pool) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

// FetchUnpublished returns up to limit outbox events that have not yet been
// forwarded to the broker, ordered oldest-first to preserve message order.
func (r *PostgresOutboxRepository) FetchUnpublished(ctx context.Context, limit int) ([]OutboxEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, job_id, payload
		FROM outbox_events
		WHERE published = false
		ORDER BY created_at ASC
		LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch unpublished: %w", err)
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var ev OutboxEvent
		if err := rows.Scan(&ev.ID, &ev.JobID, &ev.Payload); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// MarkPublished sets published = true for the given event so the relay does not forward it again.
func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE outbox_events SET published = true WHERE id = $1`, eventID,
	)
	if err != nil {
		return fmt.Errorf("mark published %s: %w", eventID, err)
	}
	return nil
}
