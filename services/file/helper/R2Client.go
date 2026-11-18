package helper

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

// GenerateUUID generates a new UUID string
func GenerateUUID() string {
	return uuid.New().String()
}

// R2Client represents a client for interacting with Cloudflare R2
type R2Client struct {
	Client     *s3.Client
	BucketName string
}

var R2ClientFactory = NewR2Client

// NewR2Client creates a new R2Client with the specified credentials
func NewR2Client() (*R2Client, error) {
	accountID := viper.GetString("cloudflare.account_id")
	accessKeyID := viper.GetString("cloudflare.access_key_id")
	secretAccessKey := viper.GetString("cloudflare.secret_access_key")
	bucketName := viper.GetString("cloudflare.bucket_name")

	if accountID == "" || accessKeyID == "" || secretAccessKey == "" || bucketName == "" {
		return nil, fmt.Errorf("missing required cloudflare r2 configuration")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		config.WithRegion("auto"),
		config.WithRetryMode(aws.RetryModeStandard),
		config.WithClientLogMode(aws.LogRetries|aws.LogRequest),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	// Create the S3 client with R2-specific endpoint
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
		o.Region = "auto"
		o.BaseEndpoint = aws.String(endpoint)
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	return &R2Client{
		Client:     client,
		BucketName: bucketName,
	}, nil
}

// GeneratePresignedURL generates a presigned URL for uploading a file to R2
func (r *R2Client) GeneratePresignedURL(fileName string, fileType string) (string, string, error) {
	// Generate a unique key for the file
	fileExt := filepath.Ext(fileName)
	uniqueKey := fmt.Sprintf("uploads/%s%s", uuid.New().String(), fileExt)

	presignClient := s3.NewPresignClient(r.Client)

	putObjectInput := &s3.PutObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(uniqueKey),
	}

	presignDuration := 15 * time.Minute
	presignedReq, err := presignClient.PresignPutObject(context.TODO(), putObjectInput,
		s3.WithPresignExpires(presignDuration))
	if err != nil {
		return "", "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}

	return presignedReq.URL, uniqueKey, nil
}

// GeneratePresignedGetURL generates a presigned URL for getting a file from R2
func (r *R2Client) GeneratePresignedGetURL(objectKey string) (string, error) {
	presignClient := s3.NewPresignClient(r.Client)

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	}

	presignDuration := 30 * time.Minute
	presignedReq, err := presignClient.PresignGetObject(context.TODO(), getObjectInput,
		s3.WithPresignExpires(presignDuration))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}

	return presignedReq.URL, nil
}

// DeleteObject deletes an object from R2
func (r *R2Client) DeleteObject(objectKey string) error {
	_, err := r.Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %v", err)
	}

	return nil
}

// GetObject retrieves an object from R2
func (r *R2Client) GetObject(objectKey string) (io.ReadCloser, error) {
	result, err := r.Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %v", err)
	}

	return result.Body, nil
}

// PutObject uploads an object to R2 using the provided key.
func (r *R2Client) PutObject(objectKey string, fileData io.Reader, fileSize int64, contentType string) error {
	_, err := r.Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws.String(r.BucketName),
		Key:           aws.String(objectKey),
		Body:          fileData,
		ContentLength: aws.Int64(fileSize),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload object: %v", err)
	}

	return nil
}

// UploadPublicFile uploads a file directly to R2 public folder and returns the public URL
func (r *R2Client) UploadPublicFile(fileName string, fileData io.Reader, fileSize int64, contentType string) (string, string, error) {
	// Generate a unique key for the file in public folder
	fileExt := filepath.Ext(fileName)
	uniqueKey := fmt.Sprintf("public/%s%s", uuid.New().String(), fileExt)

	// Upload the file directly
	_, err := r.Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws.String(r.BucketName),
		Key:           aws.String(uniqueKey),
		Body:          fileData,
		ContentLength: aws.Int64(fileSize),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to upload file: %v", err)
	}

	// Construct the public URL using CDN_URL from config
	cdnURL := viper.GetString("cdn_url")
	if cdnURL == "" {
		cdnURL = "https://cdn.qonics.io"
	}
	publicURL := fmt.Sprintf("%s/%s", cdnURL, uniqueKey)
	return publicURL, uniqueKey, nil
}
