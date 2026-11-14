package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"image-platform/api-service/internal/domain"
)

// JobResult holds the fields api-service needs to serve a processed-image download.
type JobResult struct {
	Status    string  // "PENDING" | "PROCESSING" | "COMPLETED" | "FAILED"
	ObjectKey *string // non-nil only when Status == "COMPLETED"
}

// JobResultRepository defines the single read needed to serve result downloads.
type JobResultRepository interface {
	GetResultByJobID(ctx context.Context, jobID uuid.UUID) (*JobResult, error)
}

// PostgresJobResultRepository is the pgxpool-backed implementation.
type PostgresJobResultRepository struct {
	db *pgxpool.Pool
}

// NewPostgresJobResultRepository creates a PostgresJobResultRepository backed by db.
func NewPostgresJobResultRepository(db *pgxpool.Pool) *PostgresJobResultRepository {
	return &PostgresJobResultRepository{db: db}
}

// GetResultByJobID returns the status and result_object_key for the given job,
// or domain.ErrNotFound if no row matches.
func (r *PostgresJobResultRepository) GetResultByJobID(ctx context.Context, jobID uuid.UUID) (*JobResult, error) {
	jr := &JobResult{}
	err := r.db.QueryRow(ctx,
		`SELECT status::text, result_object_key FROM jobs WHERE id = $1`, jobID,
	).Scan(&jr.Status, &jr.ObjectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job result %s: %w", jobID, err)
	}
	return jr, nil
}
