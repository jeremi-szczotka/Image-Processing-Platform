// services/job-service/internal/domain/job.go
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

var ValidOperations = map[string]bool{
	"resize":         true,
	"compress":       true,
	"convert":        true,
	"grayscale":      true,
	"edge_detection": true,
}

type Job struct {
	ID                uuid.UUID      `json:"id"`
	ImageID           uuid.UUID      `json:"image_id"`
	Operation         string         `json:"operation"`
	Parameters        map[string]any `json:"parameters"`
	Status            string         `json:"status"`
	ResultObjectKey   *string        `json:"result_object_key,omitempty"`
	ResultContentType *string        `json:"result_content_type,omitempty"`
	ResultFileSize    *int64         `json:"result_file_size,omitempty"`
	AttemptCount      int            `json:"attempt_count"`
	ErrorMessage      *string        `json:"error_message,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	StartedAt         *time.Time     `json:"started_at,omitempty"`
	FinishedAt        *time.Time     `json:"finished_at,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
