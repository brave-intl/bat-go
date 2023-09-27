package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	awslogging "github.com/aws/smithy-go/logging"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/rs/zerolog"
)

// S3GetObjectAPI - interface to allow for a GetObject mock
type S3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3UploadAPI defines
type S3UploadAPI interface {
	CreateMultipartUpload(ctx context.Context, params *s3.CreateMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(ctx context.Context, params *s3.UploadPartInput, optFns ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(ctx context.Context, params *s3.CompleteMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(ctx context.Context, params *s3.AbortMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
}

type S3UploadConfig struct {
	Bucket      string
	ContentType string
	PartSize    int64
}

// Client defines the aws client.
type Client struct {
	S3GetObjectAPI
	S3UploadAPI
}

// NewClient creates a new aws client instance.
func NewClient(cfg aws.Config, optFns ...func(*s3.Options)) *Client {
	f := func(o *s3.Options) {
		o.UsePathStyle = true
	}

	c := s3.NewFromConfig(cfg, f)
	return &Client{
		S3GetObjectAPI: c,
		S3UploadAPI:    c,
	}
}

// BaseAWSConfig return an aws.Config with region and logger.
// Default region is us-west-2.
func BaseAWSConfig(ctx context.Context, logger *zerolog.Logger) (aws.Config, error) {
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok || len(region) == 0 {
		region = "us-west-2"
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service string, region string,
		options ...interface{}) (aws.Endpoint, error) {
		if os.Getenv("ENV") == "local" {
			return aws.Endpoint{
				PartitionID:   "aws",
				URL:           os.Getenv("AWS_ENDPOINT"),
				SigningRegion: "us-east-1",
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	// aws config
	return config.LoadDefaultConfig(
		ctx,
		config.WithLogger(&appLogger{logger}),
		config.WithRegion(region),
		config.WithEndpointResolverWithOptions(customResolver),
	)
}

type appLogger struct {
	*zerolog.Logger
}

// Logf - implement smithy-go/logging.Logger
func (al *appLogger) Logf(classification awslogging.Classification, format string, v ...interface{}) {
	al.Debug().Msg(fmt.Sprintf(format, v...))
}
