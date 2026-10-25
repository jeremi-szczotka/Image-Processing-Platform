package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioStorage implements Storage using the MinIO (S3-compatible) object store.
type MinioStorage struct {
	client *minio.Client
}

// NewMinioStorage creates a MinioStorage connected to the given endpoint using static credentials.
// useSSL controls whether the connection uses TLS.
func NewMinioStorage(endpoint, accessKey, secretKey string, useSSL bool) (*MinioStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &MinioStorage{client: client}, nil
}

// EnsureBuckets creates required buckets if they do not exist.
// Called once at application startup, not part of the Storage interface.
func (s *MinioStorage) EnsureBuckets(ctx context.Context) error {
	for _, bucket := range []string{BucketOriginal, BucketProcessed} {
		exists, err := s.client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("check bucket %q: %w", bucket, err)
		}
		if !exists {
			if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("create bucket %q: %w", bucket, err)
			}
		}
	}
	return nil
}

// Upload stores the contents of reader as objectKey in bucket.
// size must be the exact byte length of reader; the MinIO client uses it for the Content-Length header.
func (s *MinioStorage) Upload(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload %s/%s: %w", bucket, objectKey, err)
	}
	return nil
}

// PresignedDownloadURL returns a time-limited URL the client can GET directly from MinIO
// without any further server involvement. The URL expires after expiry.
func (s *MinioStorage) PresignedDownloadURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, bucket, objectKey, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presign %s/%s: %w", bucket, objectKey, err)
	}
	return u.String(), nil
}
