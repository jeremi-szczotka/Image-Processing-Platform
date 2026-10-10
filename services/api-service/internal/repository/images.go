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

// ImageRepository defines database operations for images.
type ImageRepository interface {
	Create(ctx context.Context, img *domain.Image) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Image, error)
}

type PostgresImageRepository struct {
	db *pgxpool.Pool
}

func NewPostgresImageRepository(db *pgxpool.Pool) *PostgresImageRepository {
	return &PostgresImageRepository{db: db}
}

func (r *PostgresImageRepository) Create(ctx context.Context, img *domain.Image) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO images (id, original_filename, object_key, content_type, file_size, width, height)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		img.ID, img.OriginalFilename, img.ObjectKey,
		img.ContentType, img.FileSize, img.Width, img.Height,
	)
	if err != nil {
		return fmt.Errorf("insert image: %w", err)
	}
	return nil
}

func (r *PostgresImageRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Image, error) {
	img := &domain.Image{}
	err := r.db.QueryRow(ctx, `
		SELECT id, original_filename, object_key, content_type, file_size, width, height, created_at
		FROM images WHERE id = $1`, id,
	).Scan(
		&img.ID, &img.OriginalFilename, &img.ObjectKey,
		&img.ContentType, &img.FileSize, &img.Width, &img.Height, &img.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get image %s: %w", id, err)
	}
	return img, nil
}
