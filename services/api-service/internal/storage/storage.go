package storage

import (
	"context"
	"io"
	"time"
)

// Storage defines the file-storage operations the API service depends on.
type Storage interface {
	Upload(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error
	PresignedDownloadURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error)
}

const (
	BucketOriginal  = "images-original"
	BucketProcessed = "images-processed"
)
