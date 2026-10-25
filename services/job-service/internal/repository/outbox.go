// services/job-service/internal/repository/outbox.go
package repository

import (
	"context"

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
	panic("not implemented — see Task 5")
}

func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	panic("not implemented — see Task 5")
}
