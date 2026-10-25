// Package domain defines the core types and sentinel errors shared across
// the job-service layers.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by repository lookups when the requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ValidOperations is the set of accepted image-processing operation names.
// A request that names an operation not in this map is rejected with 400.
var ValidOperations = map[string]bool{
	"resize":         true,
	"compress":       true,
	"convert":        true,
	"grayscale":      true,
	"edge_detection": true,
}

// Job represents an image-processing job and tracks its full lifecycle
// from initial creation through worker execution to final success or failure.
type Job struct {
	ID         uuid.UUID      `json:"id"`
	ImageID    uuid.UUID      `json:"image_id"`
	Operation  string         `json:"operation"`
	Parameters map[string]any `json:"parameters"`
	Status     string         `json:"status"`
	// ResultObjectKey is the MinIO object key of the processed image; nil until the job completes successfully.
	ResultObjectKey *string `json:"result_object_key,omitempty"`
	// ResultContentType is the MIME type of the processed image; nil until the job completes successfully.
	ResultContentType *string `json:"result_content_type,omitempty"`
	// ResultFileSize is the byte size of the processed image; nil until the job completes successfully.
	ResultFileSize *int64 `json:"result_file_size,omitempty"`
	// AttemptCount tracks how many times a worker has attempted to process this job.
	AttemptCount int `json:"attempt_count"`
	// ErrorMessage captures the most recent worker error; nil when the job is pending or succeeded.
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	// StartedAt is the time a worker first picked up the job; nil while the job is still pending.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// FinishedAt is the time processing concluded (success or permanent failure); nil while in progress.
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
