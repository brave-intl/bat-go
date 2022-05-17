package payments

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	appaws "github.com/brave-intl/bat-go/utils/nitro/aws"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type awsClient struct{}

// RetrieveSecrets - implements secret discovery for payments service
func (ac *awsClient) RetrieveSecrets(ctx context.Context, uri string) ([]byte, error) {
	logger := logging.Logger(ctx, "awsClient.RetrieveSecrets")

	// decrypt the aws region
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		err := errors.New("empty aws region")
		logger.Error().Err(err).Str("region", region).Msg("aws region")
		return nil, err
	}

	// decrypt the s3 object with the kms key.
	s3URI, ok := ctx.Value(appctx.SecretsURICTXKey).(string)
	if !ok {
		err := errors.New("empty secrets uri")
		logger.Error().Err(err).Str("uri", uri).Msg("secrets location")
		return nil, err
	}
	parts := strings.Split(s3URI, "/")
	// get bucket and object from url
	bucket := parts[len(parts)-2]
	object := parts[len(parts)-1]

	// decrypt the s3 object with the kms key.
	kmsKey, ok := ctx.Value(appctx.PaymentsKMSWrapperCTXKey).(string)
	if !ok {
		err := errors.New("empty kms wrapper key")
		logger.Error().Err(err).Str("kmsKey", kmsKey).Msg("kms key")
		return nil, err
	}

	// get proxy address for outbound
	egressProxyAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("failed to get egress proxy for qldb")
	}

	logger.Debug().
		Str("kms", kmsKey).
		Str("egress", egressProxyAddr).
		Str("bucket", bucket).
		Str("object", object).
		Str("region", region).
		Msg("secrets location details")

	cfg, err := appaws.NewAWSConfig(egressProxyAddr, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws configuration: %w", err)
	}
	awsCfg, ok := cfg.(aws.Config)
	if !ok {
		return nil, fmt.Errorf("invalid aws configuration: %w", err)
	}

	algo := "AES256"
	client := s3.NewFromConfig(awsCfg)
	input := &s3.GetObjectInput{
		Bucket:               &bucket,
		Key:                  &object,
		SSECustomerAlgorithm: &algo,   // kms algorithm
		SSECustomerKey:       &kmsKey, // kms key to use for decrypt
	}

	secretsResponse, err := client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	data, err := ioutil.ReadAll(secretsResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}

	return data, nil
}
