// services/job-service/internal/repository/outbox.go
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxEvent struct {
	ID      uuid.UUID
	JobID   uuid.UUID
	Payload []byte
}

type OutboxRepository interface {
	FetchUnpublished(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
}

type PostgresOutboxRepository struct {
	db *pgxpool.Pool
}

func NewPostgresOutboxRepository(db *pgxpool.Pool) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

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

func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE outbox_events SET published = true WHERE id = $1`, eventID,
	)
	if err != nil {
		return fmt.Errorf("mark published %s: %w", eventID, err)
	}
	return nil
}
