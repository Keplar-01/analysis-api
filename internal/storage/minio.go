package storage

import (
	"context"
	"io"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	BucketSourceCodes      = "source-codes"
	BucketAnalysisArtifacts = "analysis-artifacts"
)

type MinIOClient struct {
	client *minio.Client
}

func NewMinIOClient(endpoint, accessKey, secretKey string, useSSL bool) (*MinIOClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	for _, bucket := range []string{BucketSourceCodes, BucketAnalysisArtifacts} {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			log.Printf("warning: cannot check bucket %s: %v", bucket, err)
			continue
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				log.Printf("warning: cannot create bucket %s: %v", bucket, err)
			}
		}
	}

	return &MinIOClient{client: client}, nil
}

func (m *MinIOClient) Upload(ctx context.Context, bucket, objectName string, reader io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (m *MinIOClient) Download(ctx context.Context, bucket, objectName string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (m *MinIOClient) HealthCheck(ctx context.Context) error {
	_, err := m.client.BucketExists(ctx, BucketSourceCodes)
	return err
}
