package payments

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3crypto"
)

type awsClient struct{}

// RetrieveSecrets - implements secret discovery for payments service
func (ac *awsClient) RetrieveSecrets(ctx context.Context, uri string) ([]byte, error) {
	logger := logging.Logger(ctx, "awsClient.RetrieveSecrets")

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

	// create session
	sess := session.Must(session.NewSession())
	// create kms client
	kmsClient := kms.New(sess)

	// Create a CryptoRegistry and register the algorithms you wish to use for decryption
	cr := s3crypto.NewCryptoRegistry()

	if err := s3crypto.RegisterAESGCMContentCipher(cr); err != nil {
		return nil, fmt.Errorf("failed to register aes as a content cipher: %w", err)
	}

	if err := s3crypto.RegisterKMSContextWrapWithAnyCMK(cr, kmsClient); err != nil {
		return nil, fmt.Errorf("failed to register context wrap with cmk: %w", err)
	}

	// Create a decryption client to decrypt artifacts
	decryptionClient, err := s3crypto.NewDecryptionClientV2(sess, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to create decryption client: %w", err)
	}
	getObject, err := decryptionClient.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(object),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	data, err := ioutil.ReadAll(getObject.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}

	return data, nil
}
