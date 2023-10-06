package aws

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"golang.org/x/net/http2"

	smithy "github.com/aws/smithy-go/logging"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
)

// NewAWSConfig creates a new AWS SDK config that communicates via an HTTP
// proxy listening on a vsock address, it automatically retrieves any EC2
// role credentials of the instance hosting the enclave
func NewAWSConfig(ctx context.Context, proxyAddr string, region string) (aws.Config, error) {
	logger := logging.Logger(ctx, "aws.NewAWSConfig")

	logger.Info().
		Str("proxyAddr", proxyAddr).
		Str("region", region).
		Msg("setting up new aws config")

	var client http.Client
	tr := nitro.NewProxyRoundTripper(ctx, proxyAddr)

	logger.Info().
		Str("transport", fmt.Sprintf("%+v", tr)).
		Msg("transport is setup")
	// So client makes HTTP/2 requests
	err := http2.ConfigureTransport(tr.(*http.Transport))
	if err != nil {
		log.Panic(err)
	}

	client = http.Client{
		Transport: tr,
	}

	var applicationLogger smithy.Logger

	if logWriter, ok := ctx.Value(appctx.LogWriterCTXKey).(io.Writer); ok {
		applicationLogger = smithy.NewStandardLogger(logWriter)
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithHTTPClient(&client),
		config.WithRegion("us-west-2"),
		config.WithLogger(applicationLogger),
		config.WithClientLogMode(aws.LogRetries|aws.LogRequest),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("unable to load SDK config, %v", err)
	}

	provider := ec2rolecreds.New(func(options *ec2rolecreds.Options) {
		options.Client = imds.NewFromConfig(cfg)
	})

	return config.LoadDefaultConfig(context.TODO(),
		config.WithHTTPClient(&client),
		config.WithRegion(region),
		config.WithCredentialsProvider(provider),
		config.WithLogger(applicationLogger),
		config.WithClientLogMode(aws.LogRetries|aws.LogRequest),
	)
}
