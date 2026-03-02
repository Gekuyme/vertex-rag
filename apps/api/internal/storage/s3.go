package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	bucket   string
	s3Client *s3.Client
	uploader *manager.Uploader
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

	client := &Client{
		bucket:   cfg.Bucket,
		s3Client: s3Client,
		uploader: manager.NewUploader(s3Client),
	}

	if err := client.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *Client) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := c.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("upload object: %w", err)
	}

	return nil
}

func (c *Client) ensureBucket(ctx context.Context) error {
	_, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err == nil {
		return nil
	}

	_, createErr := c.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(c.bucket)})
	if createErr != nil {
		if strings.Contains(strings.ToLower(createErr.Error()), "bucketalreadyownedbyyou") {
			return nil
		}
		return fmt.Errorf("create bucket: %w", createErr)
	}

	return nil
}

func scheme(useSSL bool) string {
	if useSSL {
		return "https"
	}
	return "http"
}

func normalizeEndpoint(endpoint string) string {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		parsedURL, err := url.Parse(endpoint)
		if err == nil {
			return parsedURL.Host
		}
	}

	return endpoint
}
