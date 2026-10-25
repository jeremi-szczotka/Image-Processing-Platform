// services/job-service/internal/repository/jobs.go
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"image-platform/job-service/internal/domain"
)

var ErrImageNotFound = errors.New("image not found")

type JobRepository interface {
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
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	paramsJSON, err := json.Marshal(job.Parameters)
	if err != nil {
		return fmt.Errorf("marshal parameters: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO jobs (id, image_id, operation, parameters, status)
		VALUES ($1, $2, $3, $4, 'PENDING')
		RETURNING created_at, updated_at`,
		job.ID, job.ImageID, job.Operation, paramsJSON,
	).Scan(&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrImageNotFound
		}
		return fmt.Errorf("insert job: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (job_id, payload) VALUES ($1, $2)`,
		job.ID, payload,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	job.Status = "PENDING"
	return nil
}

func (r *PostgresJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	job := &domain.Job{}
	var paramsJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, image_id, operation, parameters, status,
		       result_object_key, result_content_type, result_file_size,
		       attempt_count, error_message, created_at, started_at, finished_at, updated_at
		FROM jobs WHERE id = $1`, id,
	).Scan(
		&job.ID, &job.ImageID, &job.Operation, &paramsJSON, &job.Status,
		&job.ResultObjectKey, &job.ResultContentType, &job.ResultFileSize,
		&job.AttemptCount, &job.ErrorMessage, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", id, err)
	}
	if err := json.Unmarshal(paramsJSON, &job.Parameters); err != nil {
		return nil, fmt.Errorf("unmarshal parameters: %w", err)
	}
	return job, nil
}
