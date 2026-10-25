// services/job-service/internal/repository/jobs.go
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"image-platform/job-service/internal/domain"
)

// ErrImageNotFound is returned when the referenced image does not exist (FK violation).
var ErrImageNotFound = errors.New("image not found")

type JobRepository interface {
	// CreateWithOutbox inserts the job and an outbox_event in a single transaction.
	CreateWithOutbox(ctx context.Context, job *domain.Job, payload []byte) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error)
}

type PostgresJobRepository struct {
	db *pgxpool.Pool
}

func NewPostgresJobRepository(db *pgxpool.Pool) *PostgresJobRepository {
	return &PostgresJobRepository{db: db}
}

func (r *PostgresJobRepository) CreateWithOutbox(ctx context.Context, job *domain.Job, payload []byte) error {
	panic("not implemented — see Task 5")
}

func (r *PostgresJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	panic("not implemented — see Task 5")
}
