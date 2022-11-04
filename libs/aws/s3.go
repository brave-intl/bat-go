package aws

import (
	"context"
	"fmt"

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

// Client defines the aws client.
type Client struct {
	S3GetObjectAPI
}

// NewClient creates a new aws client instance.
func NewClient(cfg aws.Config) (*Client, error) {
	return &Client{
		S3GetObjectAPI: s3.NewFromConfig(cfg),
	}, nil
}

// BaseAWSConfig return an aws.Config with region and logger.
// Default region is us-west-2.
func BaseAWSConfig(ctx context.Context, logger *zerolog.Logger) (aws.Config, error) {
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok || len(region) == 0 {
		region = "us-west-2"
	}
	// aws config
	return config.LoadDefaultConfig(
		ctx,
		config.WithLogger(&appLogger{logger}),
		config.WithRegion(region))
}

type appLogger struct {
	*zerolog.Logger
}

// Logf - implement smithy-go/logging.Logger
func (al *appLogger) Logf(classification awslogging.Classification, format string, v ...interface{}) {
	al.Debug().Msg(fmt.Sprintf(format, v...))
}
