// Package storage defines the object-storage abstraction and well-known bucket constants used by the api-service.
package storage

import (
	"context"
	"io"
	"time"
)

// Storage abstracts object-store operations so that handlers do not depend on MinIO directly.
// MinioStorage is the production implementation.
type Storage interface {
	// Upload stores reader's contents as objectKey in bucket. size must equal the exact byte length of reader.
	Upload(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error
	// PresignedDownloadURL returns a time-limited URL that a client can GET directly from object storage,
	// valid for the duration specified by expiry.
	PresignedDownloadURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error)
}

const (
	// BucketOriginal is the MinIO bucket that holds raw uploaded images.
	BucketOriginal  = "images-original"
	// BucketProcessed is the MinIO bucket that holds images after worker processing.
	BucketProcessed = "images-processed"
)
