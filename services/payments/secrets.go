package payments

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"filippo.io/age"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	"github.com/hashicorp/vault/shamir"
)

// createAttestationDocument will create an attestation document and return the private key and
// attestation document which is attesting over the userData supplied
func createAttestationDocument(ctx context.Context, userData []byte) (crypto.PrivateKey, []byte, error) {
	// create a one time use nonce
	nonce, err := createAttestationNonce(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create attestation nonce: %w", err)
	}

	// create rsa private/public key pair for the document
	privateKey, publicKey, err := createAttestationKey(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create attestation key: %w", err)
	}

	publicKeyMarshaled, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode public key bytes for attestation: %w", err)
	}

	// attest to the document with passed in user data
	document, err := nitro.Attest(ctx, nonce, userData, publicKeyMarshaled)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create attestation document: %w", err)
	}
	return privateKey, document, nil

}

// createAttestationKey is a helper to create an RSA key for nitro attestation document
// such that kms will encrypt the results to this created key
func createAttestationKey(ctx context.Context) (crypto.PrivateKey, crypto.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rsa private key for attestation: %w", err)
	}
	return privateKey, privateKey.Public(), nil
}

// createAttestationNonce is a helper to create a random nonce for the attestation document
func createAttestationNonce(ctx context.Context) ([]byte, error) {
	nonce := make([]byte, 64)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce for attestation: %w", err)
	}
	return nonce, nil
}

// nitroAwsCfg is a helper to get an aws config for nitro applications
func nitroAwsCfg(ctx context.Context) (aws.Config, error) {
	// get proxy address for outbound
	egressAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		egressAddr = ":1234"
	}

	// download the configuration from s3 bucket/object
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	return nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
}

// fetchSecrets will take an s3 bucket/object and fetch the configuration and store the
// ciphertext on the service for decryption later
func (s *Service) fetchSecrets(ctx context.Context, bucket, object string) error {
	awsCfg, err := nitroAwsCfg(ctx)
	if err != nil {
		return fmt.Errorf("failed to create aws config for s3 client: %w", err)
	}

	// get the secrets configurations from s3
	secretsResponse, err := s3.NewFromConfig(awsCfg).GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(object),
	})
	if err != nil {
		return fmt.Errorf("failed to get secrets from s3: %w", err)
	}

	// we are not able to decrypt secretsCiphertext until all operator shares are available
	s.secretsCiphertext, err = io.ReadAll(secretsResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read secrets bytes: %w", err)
	}

	return nil
}

// enoughOperatorShares informs the caller if there are enough operator shares present to attempt a decrypt
func (s *Service) enoughOperatorShares(ctx context.Context, required int) bool {
	if len(s.keyShares) > required { // TODO: configurable in future, right now need two shares
		return true
	}
	return false
}

var (
	errNoSecretsCiphertext = errors.New("failed to get service configuration ciphertext")
)

// configureSecrets takes the ciphertext configuration from fetchSecrets, then decrypts it with the keyshares
// from fetchOperatorShares then stores the values in the configuration map
func (s *Service) configureSecrets(ctx context.Context) error {
	// do we have secrets downloaded?
	if len(s.secretsCiphertext) < 1 {
		return errNoSecretsCiphertext
	}

	// decrypt configuration ciphertext
	secrets, err := s.decryptSecrets(ctx)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// store conf on service
	s.secrets = secrets
	return nil
}

// fetchOperatorShares will take an s3 bucket and fetch all of the operator shares and store them
func (s *Service) fetchOperatorShares(ctx context.Context, bucket string) error {
	// clear out all keyshares and start over, we will be downloading ALL shares from the s3 bucket
	s.keyShares = [][]byte{}

	// get the aws configuration
	awsCfg, err := nitroAwsCfg(ctx)
	if err != nil {
		return fmt.Errorf("failed to create aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)

	// list all objects in the bucket prefixed with operator-share
	shareObjects, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Prefix: aws.String("operator-share"),
		Bucket: aws.String(bucket),
	})

	// for each share object, get it, attempt to decrypt and append to keyShares
	for _, shareObject := range shareObjects.Contents {
		// use kms encrypt key arn on service to decrypt each file
		shareResponse, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    shareObject.Key, // the share object key for this iteration
		})
		if err != nil {
			return fmt.Errorf("failed to get operator share from s3: %w", err)
		}

		data, err := ioutil.ReadAll(shareResponse.Body)
		if err != nil {
			return fmt.Errorf("failed to read operator share from s3 response: %w", err)
		}

		privateKey, document, err := createAttestationDocument(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to create attestation document: %w", err)
		}

		// decrypt with kms key that only enclave can decrypt with
		decryptOutput, err := kms.NewFromConfig(awsCfg).Decrypt(ctx, &kms.DecryptInput{
			CiphertextBlob:      data,
			EncryptionAlgorithm: kmsTypes.EncryptionAlgorithmSpecSymmetricDefault,
			KeyId:               aws.String(s.kmsDecryptKeyArn),
			Recipient: &kmsTypes.RecipientInfo{
				AttestationDocument:    document,                                       // attestation document
				KeyEncryptionAlgorithm: kmsTypes.KeyEncryptionMechanismRsaesOaepSha256, // how to decrypt
			},
		})
		if err != nil {
			return fmt.Errorf("failed to decrypt object with kms: %w", err)
		}

		plaintext, err := nitro.Decrypt(privateKey.(*rsa.PrivateKey), decryptOutput.CiphertextForRecipient)
		if err != nil {
			return fmt.Errorf("failed to decrypt the ciphertext for recipient from kms: %w", err)
		}

		// store the decrypted keyShares on the service as [][]byte for later
		s.keyShares = append(s.keyShares, plaintext)
	}

	return nil
}

// decryptSecrets combines the shamir shares stored on the service instance and decrypts the ciphertext
// returning a map of secret values from the configuration
func (s *Service) decryptSecrets(ctx context.Context) (map[string]string, error) {
	var logger = logging.Logger(ctx, "payments")
	// combine the service configured key shares
	privateKey, err := shamir.Combine(s.keyShares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine keyShares: %w", err)
	}
	logger.Info().Msgf("share 1: %s", string(s.keyShares[0]))

	logger.Info().Msgf("share 2: %s", string(s.keyShares[1]))

	logger.Info().Msgf("age identity key: %+v", string(privateKey))

	identity, err := age.ParseX25519Identity(string(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key bytes for secret decryption: %w", err)
	}

	buf := bytes.NewBuffer(s.secretsCiphertext)

	r, err := age.Decrypt(buf, identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the secrets ciphertext: %w", err)
	}

	var output = map[string]string{}
	if err := json.NewDecoder(r).Decode(output); err != nil {
		return nil, fmt.Errorf("failed to json decode the secrets: %w", err)
	}

	return output, nil
}
