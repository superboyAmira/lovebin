package s3

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3 interface for dependency injection
type S3 interface {
	Upload(ctx context.Context, bucket, key string, body io.Reader) (string, error)
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, key string) error
}

type s3Impl struct {
	client *s3.Client
	bucket string
}

// Config holds S3 configuration
type Config struct {
	Region   string
	Bucket   string
	Endpoint string // Optional, for local S3-compatible services
	AccessKeyID string
	SecretAccessKey string
}

// Init initializes the S3 module
func Init(ctx context.Context, cfg Config) (S3, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	// Use custom credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // For S3-compatible services like MinIO
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)

	return &s3Impl{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

func (s *s3Impl) Upload(ctx context.Context, bucket, key string, body io.Reader) (string, error) {
	bucketName := bucket
	if bucketName == "" {
		bucketName = s.bucket
	}

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return "", err
	}

	return key, nil
}

func (s *s3Impl) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	bucketName := bucket
	if bucketName == "" {
		bucketName = s.bucket
	}

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	return result.Body, nil
}

func (s *s3Impl) Delete(ctx context.Context, bucket, key string) error {
	bucketName := bucket
	if bucketName == "" {
		bucketName = s.bucket
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	return err
}

