package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/worker/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	bucket   string
	s3Client *s3.Client
}

func NewS3Client(ctx context.Context, cfg config.S3Config) (*Client, error) {
	endpoint := normalizeEndpoint(cfg.Endpoint)
	resolver := s3.EndpointResolverFromURL(fmt.Sprintf("%s://%s", scheme(cfg.UseSSL), endpoint))
	awsCfg, err := awsConfig.LoadDefaultConfig(
		ctx,
		awsConfig.WithRegion(cfg.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = true
		options.EndpointResolver = resolver
	})

	return &Client{
		bucket:   cfg.Bucket,
		s3Client: s3Client,
	}, nil
}

func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	object, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}

	return object.Body, nil
}

func scheme(useSSL bool) string {
	if useSSL {
		return "https"
	}
	return "http"
}

func normalizeEndpoint(endpoint string) string {
	cleanEndpoint := strings.TrimSpace(endpoint)
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "http://")
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "https://")
	return cleanEndpoint
}
