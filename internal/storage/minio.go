package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage defines the interface for interacting with object storage.
type Storage interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	ListKeys(ctx context.Context, prefix string) ([]string, error)
}

// MinIOStorage implements the Storage interface using MinIO/S3.
type MinIOStorage struct {
	client *minio.Client
	bucket string
}

// NewMinIOStorage creates a new MinIO client and initializes the storage provider.
func NewMinIOStorage(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIOStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating minio client: %w", err)
	}

	return &MinIOStorage{
		client: client,
		bucket: bucket,
	}, nil
}

// EnsureBucket checks if the configured bucket exists and creates it if it doesn't.
func (s *MinIOStorage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}
	}
	return nil
}

// Upload transfers data from a reader to a specific key in object storage.
func (s *MinIOStorage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("uploading object: %w", err)
	}
	return nil
}

// Download retrieves an object from storage as a read closer.
func (s *MinIOStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("downloading object: %w", err)
	}
	return obj, nil
}

// Delete removes an object from storage by its key.
func (s *MinIOStorage) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}
	return nil
}

// ListKeys returns all object keys that match the given prefix.
func (s *MinIOStorage) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("listing objects: %w", obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}
