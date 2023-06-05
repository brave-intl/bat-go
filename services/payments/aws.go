// Package payments is the service that executes payments
package payments

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type awsClient struct{}

func (ac *awsClient) IsReady(ctx context.Context) bool {
	logger := logging.Logger(ctx, "awsClient.IsReady")
	// decrypt the aws region
	region, regionOK := ctx.Value(appctx.AWSRegionCTXKey).(string)
	// decrypt the s3 object with the kms key.
	s3uri, s3OK := ctx.Value(appctx.SecretsURICTXKey).(string)
	// decrypt the s3 object with the kms key.
	kmsKey, kmsOK := ctx.Value(appctx.PaymentsKMSWrapperCTXKey).(string)
	// get proxy address for outbound
	egressAddr, egressOK := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if regionOK && s3OK && kmsOK && egressOK {
		return true
	}
	logger.Warn().
		Str("region", region).
		Str("s3uri", s3uri).
		Str("kmsKey", kmsKey).
		Str("egressAddr", egressAddr).
		Msg("service is not configured to get secrets")
	return false
}

// RetrieveSecrets - implements secret discovery for payments service
func (ac *awsClient) RetrieveSecrets(ctx context.Context /*uri string*/) ([]byte, error) {
	logger := logging.Logger(ctx, "awsClient.RetrieveSecrets")

	// check if client is ready
	if !ac.IsReady(ctx) {
		err := errors.New("client is not yet configured")
		logger.Error().Err(err).Msg("client needs configuration")
		return nil, err
	}

	// decrypt the aws region
	region, _ := ctx.Value(appctx.AWSRegionCTXKey).(string)
	// decrypt the s3 object with the kms key.
	s3URI, _ := ctx.Value(appctx.SecretsURICTXKey).(string)
	parts := strings.Split(s3URI, "/")
	// get bucket and object from url
	bucket := parts[len(parts)-2]
	object := parts[len(parts)-1]

	// decrypt the s3 object with the kms key.
	kmsKey, _ := ctx.Value(appctx.PaymentsKMSWrapperCTXKey).(string)

	// get proxy address for outbound
	egressProxyAddr, _ := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)

	logger.Debug().
		Str("kms", kmsKey).
		Str("egress", egressProxyAddr).
		Str("bucket", bucket).
		Str("object", object).
		Str("region", region).
		Msg("secrets location details")

	awsCfg, err := appaws.NewAWSConfig(ctx, egressProxyAddr, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws configuration: %w", err)
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
