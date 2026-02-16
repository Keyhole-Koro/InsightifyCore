package artifact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type S3Store struct {
	client     *minio.Client
	bucketName string
	region     string
	initOnce   sync.Once
	initErr    error
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("s3 endpoint is required")
	}
	access := strings.TrimSpace(cfg.AccessKey)
	secret := strings.TrimSpace(cfg.SecretKey)
	if access == "" || secret == "" {
		return nil, fmt.Errorf("s3 access key and secret key are required")
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: cfg.UseSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}

	return &S3Store{
		client:     client,
		bucketName: bucket,
		region:     region,
	}, nil
}

func (s *S3Store) ensureBucket(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("store is nil")
	}
	s.initOnce.Do(func() {
		exists, err := s.client.BucketExists(ctx, s.bucketName)
		if err != nil {
			s.initErr = err
			return
		}
		if exists {
			return
		}
		s.initErr = s.client.MakeBucket(ctx, s.bucketName, minio.MakeBucketOptions{Region: s.region})
	})
	return s.initErr
}

func (s *S3Store) Put(ctx context.Context, runID, path string, content []byte) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if err := s.ensureBucket(ctx); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}
	if content == nil {
		content = []byte{}
	}

	key := objectKey(runID, path)
	_, err := s.client.PutObject(ctx, s.bucketName, key, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	return err
}

func (s *S3Store) Get(ctx context.Context, runID, path string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, fmt.Errorf("ensure bucket: %w", err)
	}

	key := objectKey(runID, path)
	obj, err := s.client.GetObject(ctx, s.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" || errResp.Code == "NoSuchBucket" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

func (s *S3Store) List(ctx context.Context, runID string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, fmt.Errorf("ensure bucket: %w", err)
	}

	prefix := strings.TrimSuffix(runID, "/") + "/"
	paths := make([]string, 0, 32)
	for obj := range s.client.ListObjects(ctx, s.bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		if obj.Key == "" {
			continue
		}
		paths = append(paths, strings.TrimPrefix(obj.Key, prefix))
	}
	sort.Strings(paths)
	return paths, nil
}

func (s *S3Store) GetURL(ctx context.Context, runID, path string) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("store is nil")
	}
	key := objectKey(runID, path)
	// Expiry: 1 hour
	u, err := s.client.PresignedGetObject(ctx, s.bucketName, key, time.Hour, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func objectKey(runID, path string) string {
	normalized := strings.TrimLeft(strings.TrimSpace(path), "/")
	return strings.TrimSpace(runID) + "/" + normalized
}
