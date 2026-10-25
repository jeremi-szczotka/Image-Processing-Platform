// Package domain defines the core data types and sentinel errors for the api-service.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by repository methods when no matching record exists in the database.
var ErrNotFound = errors.New("not found")

// Image represents the metadata for an uploaded image stored in the platform.
type Image struct {
	ID               uuid.UUID `json:"id"`
	OriginalFilename string    `json:"original_filename"`
	// ObjectKey is the path of the object inside the MinIO bucket (e.g. "<uuid>/original.jpg").
	// It is intentionally excluded from JSON responses because it is an internal storage detail.
	ObjectKey        string    `json:"-"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	CreatedAt        time.Time `json:"created_at"`
}
