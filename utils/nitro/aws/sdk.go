package aws

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"golang.org/x/net/http2"

	"github.com/brave-intl/bat-go/utils/nitro"
)

// NewAWSConfig creates a new AWS SDK config that communicates via an HTTP
// proxy listening on a vsock address, it automatically retrieves any EC2
// role credentials of the instance hosting the enclave
func NewAWSConfig(proxyAddr string, region string) (config.Config, error) {
	var client http.Client
	tr := nitro.NewProxyRoundTripper(proxyAddr)

	// So client makes HTTP/2 requests
	err := http2.ConfigureTransport(tr.(*http.Transport))
	if err != nil {
		log.Panic(err)
	}

	client = http.Client{
		Transport: tr,
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithHTTPClient(&client),
		config.WithRegion("us-west-2"),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config, %v", err)
	}

	provider := ec2rolecreds.New(func(options *ec2rolecreds.Options) {
		options.Client = imds.NewFromConfig(cfg)
	})

	return config.LoadDefaultConfig(context.TODO(),
		config.WithHTTPClient(&client),
		config.WithRegion(region),
		config.WithCredentialsProvider(provider),
	)
}
