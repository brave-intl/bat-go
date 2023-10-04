package payments

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
	"time"

	nitro_eclave_attestation_document "github.com/veracruz-project/go-nitro-enclave-attestation-document"

	"encoding/hex"
	"encoding/json"
	"encoding/pem"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/hashicorp/vault/shamir"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	appsrv "github.com/brave-intl/bat-go/libs/service"
)

type compatDatastore interface {
	Datastore
	wrappedQldbDriverAPI
}

// Service struct definition of payments service.
type Service struct {
	// concurrent safe
	datastore  compatDatastore
	custodians map[string]provider.Custodian
	awsCfg     aws.Config

	baseCtx          context.Context
	secretMgr        appsrv.SecretManager
	keyShares        [][]byte
	configCiphertext []byte
	config           map[string]string
	kmsDecryptKeyArn string
	sdkClient        wrappedQldbSDKClient

	publicKey []byte
	signer    paymentLib.Signator
	verifier  paymentLib.Verifier
}

func parseKeyPolicyTemplate(ctx context.Context, templateFile string) (string, string, error) {
	// perform enclave attestation
	nonce := make([]byte, 64)
	_, err := rand.Read(nonce)
	if err != nil {
		return "", "", fmt.Errorf("failed to create nonce for attestation: %w", err)
	}

	document, err := nitro.Attest(ctx, nonce, nil, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create attestation document: %w", err)
	}

	var logger = logging.Logger(ctx, "payments.configureKMSKey")
	logger.Info().Msgf("document: %+v", document)

	// parse the root certificate
	block, _ := pem.Decode([]byte(nitro.RootAWSNitroCert))

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	// parse document
	ad, err := nitro_eclave_attestation_document.AuthenticateDocument(document, *cert, true)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template attestation document: %+v", ad)
		return "", "", err
	}
	logger.Info().Msgf("digest: %+v", ad.Digest)
	logger.Info().Msgf("pcrs: %+v", ad.PCRs)

	t, err := template.ParseFiles(templateFile)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template file: %+v", templateFile)
		return "", "", err
	}

	type keyTemplateData struct {
		PCR0       string
		PCR1       string
		PCR2       string
		AWSAccount string
	}

	buf := bytes.NewBuffer([]byte{})
	if err := t.Execute(buf, keyTemplateData{
		PCR0:       hex.EncodeToString(ad.PCRs[0]),
		PCR1:       hex.EncodeToString(ad.PCRs[1]),
		PCR2:       hex.EncodeToString(ad.PCRs[2]),
		AWSAccount: os.Getenv("AWS_ACCOUNT"),
	}); err != nil {
		logger.Error().Err(err).Msgf("failed to execute template file: %+v", templateFile)
		return "", "", err
	}

	policy := buf.String()

	logger.Info().Msgf("key policy: %+v", policy)

	return policy, hex.EncodeToString(ad.PCRs[2]), nil
}

type serviceNamespaceContextKey struct{}

// fetchConfiguration will take an s3 bucket/object and fetch the configuration and store the
// ciphertext on the service for decryption later
func (s *Service) fetchConfiguration(ctx context.Context, bucket, object string) error {
	// get proxy address for outbound
	egressAddr, _ := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)

	// download the configuration from s3 bucket/object
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	awsCfg, err := nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
	if err != nil {
		return errors.New("no egress addr for payments service")
	}

	client := s3.NewFromConfig(awsCfg)

	// use kms encrypt key arn on service to decrypt
	input := &s3.GetObjectInput{
		Bucket:               aws.String(bucket),
		Key:                  aws.String(object),
		SSECustomerAlgorithm: aws.String("AES256"),           // kms algorithm
		SSECustomerKey:       aws.String(s.kmsDecryptKeyArn), // kms key to use for decrypt
	}

	secretsResponse, err := client.GetObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}

	data, err := ioutil.ReadAll(secretsResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read object: %w", err)
	}

	s.configCiphertext = data
	// store the ciphertext on the service as []byte for later
	return nil
}

// enoughShares informs the caller if there are enough operator shares present to attempt a decrypt
func (s *Service) enoughShares(ctx context.Context) bool {
	if len(s.keyShares) > 1 { // TODO: configurable in future, right now need two shares
		return true
	}
	return false
}

// configureService takes the ciphertext configuration from fetchConfiguration, then decrypts it with the keyshares
// from fetchOperatorShares then stores the values in the configuration map
func (s *Service) configureService(ctx context.Context) error {
	if len(s.configCiphertext) < 1 {
		return fmt.Errorf("failed to get service configuration ciphertext")
	}
	config, err := s.DecryptBootstrap(ctx, s.configCiphertext)
	if err != nil {
		return fmt.Errorf("failed to decrypt bootstrap configuration: %w", err)
	}
	// store conf on service
	s.config = config
	return nil
}

// fetchOperatorShares will take an s3 bucket and fetch all of the operator shares and store them
func (s *Service) fetchOperatorShares(ctx context.Context, bucket string) error {
	s.keyShares = [][]byte{} // clear out old keyshares

	// get proxy address for outbound
	egressAddr, _ := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)

	// download the configuration from s3 bucket/object
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	awsCfg, err := nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
	if err != nil {
		return errors.New("no egress addr for payments service")
	}

	client := s3.NewFromConfig(awsCfg)

	shareObjects, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})

	// for each share object, get it and append to keyShares
	for _, shareObject := range shareObjects.Contents {
		// download all objects from operator shares s3 bucket
		// use kms encrypt key arn on service to decrypt
		input := &s3.GetObjectInput{
			Bucket:               aws.String(bucket),
			Key:                  shareObject.Key,                // the share object key for this iteration
			SSECustomerAlgorithm: aws.String("AES256"),           // kms algorithm
			SSECustomerKey:       aws.String(s.kmsDecryptKeyArn), // kms key to use for decrypt
		}
		// use kms encrypt key arn on service to decrypt each file
		shareResponse, err := client.GetObject(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to get object: %w", err)
		}
		data, err := ioutil.ReadAll(shareResponse.Body)
		if err != nil {
			return fmt.Errorf("failed to read object: %w", err)
		}
		// store the decrypted keyShares on the service as [][]byte for later
		s.keyShares = append(s.keyShares, data)
	}

	return nil
}

// configureKMSEncryptionKey creates the enclave kms key which is only decrypt capable with enclave
// attestation.
func (s *Service) configureKMSEncryptionKey(ctx context.Context) error {
	// get the aws configuration loaded
	cfg := s.awsCfg
	kmsClient := kms.NewFromConfig(cfg)

	// parse the key policy
	policy, imageSHA, err := parseKeyPolicyTemplate(ctx, "/decrypt-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse decrypt policy template: %w", err)
	}

	// if the key alias already exists, pull down that particular key
	getKeyResult, err := kmsClient.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String("alias/decryption-" + imageSHA),
	})
	// If the error is that the key wasn't found, proceed. Otherwise, fail with error.
	if err != nil {
		if !strings.Contains(err.Error(), "NotFoundException") {
			return fmt.Errorf("failed to get key by alias: %w", err)
		}
	}

	if getKeyResult != nil {
		// key exists, check the key policy matches
		// if the key alias already exists, pull down that particular key
		getKeyPolicyResult, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId: getKeyResult.KeyMetadata.KeyId,
		})
		if err != nil {
			return fmt.Errorf("failed to get key by alias: %w", err)
		}

		if *getKeyPolicyResult.Policy == policy {
			// if the policy matches, we should use this key
			s.kmsDecryptKeyArn = *getKeyResult.KeyMetadata.KeyId
			return nil
		}
	}

	input := &kms.CreateKeyInput{
		Policy:                         aws.String(policy),
		BypassPolicyLockoutSafetyCheck: true,
		Tags: []kmsTypes.Tag{
			{TagKey: aws.String("Purpose"), TagValue: aws.String("settlements")},
		},
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String("alias/decryption-" + imageSHA),
		TargetKeyId: result.KeyMetadata.KeyId,
	}

	_, err = kmsClient.CreateAlias(ctx, aliasInput)
	if err != nil {
		return fmt.Errorf("failed to make key alias: %w", err)
	}

	s.kmsDecryptKeyArn = *result.KeyMetadata.KeyId
	return nil
}

// NewService creates a service using the passed datastore and clients configured from the
// environment.
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var logger = logging.Logger(ctx, "payments.NewService")

	egressAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		logger.Error().Msg("no egress addr for payments service")
		return nil, nil, errors.New("no egress addr for payments service")
	}
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	awsCfg, err := nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
	if err != nil {
		logger.Error().Msg("no egress addr for payments service")
		return nil, nil, errors.New("no egress addr for payments service")
	}

	pcrs, err := nitro.GetPCRs()
	if err != nil {
		logger.Fatal().Err(err).Msg("could not retrieve nitro PCRs")
		return nil, nil, errors.New("could not retrieve nitro PCRs")
	}

	service := &Service{
		baseCtx:   ctx,
		awsCfg:    awsCfg,
		publicKey: pcrs[2],
		signer:    nitro.Signer{},
		// FIXME need to handle past valid PCRs
		verifier: nitro.NewVerifier(map[uint][]byte{
			0: pcrs[0],
			1: pcrs[1],
			2: pcrs[2],
		}),
	}

	if err := service.configureKMSEncryptionKey(ctx); err != nil {
		logger.Error().Err(err).Msg("could not create kms secret encryption key")
		return nil, nil, errors.New("could not create kms secret encryption key")
	}

	go func() {
		_, _, err := func() (interface{}, interface{}, error) {
			// get the config object key and bucket name from environment
			configBucketName, ok := ctx.Value(appctx.EnclaveConfigBucketNameCTXKey).(string)
			if !ok {
				return nil, nil, errors.New("no configuration bucket name for payments service")
			}

			// download the configuration file, kms decrypt the file
			configObjectName, ok := ctx.Value(appctx.EnclaveConfigObjectNameCTXKey).(string)
			if !ok {
				return nil, nil, errors.New("no configuration object name for payments service")
			}

			// fetch the configuration, result will store the configuration (age ciphertext) on the service instance
			if err := service.fetchConfiguration(ctx, configBucketName, configObjectName); err != nil {
				return nil, nil, fmt.Errorf("failed to fetch configuration: %w", err)
			}

			// operator shares files
			operatorSharesBucketName, ok := ctx.Value(appctx.EnclaveOperatorSharesBucketNameCTXKey).(string)
			if !ok {
				return nil, nil, errors.New("no operator shares bucket name for payments service")
			}

			for {
				// do we have enough shares to attempt to reconstitute the key?
				if err := service.fetchOperatorShares(ctx, operatorSharesBucketName); err != nil {
					return nil, nil, fmt.Errorf("failed to fetch operator shares: %w", err)
				}
				if ok := service.enoughShares(ctx); ok {
					// yes - attempt to decrypt the file
					if err := service.configureService(ctx); err != nil {
						// fail to decrypt?  panic loudly
						return nil, nil, fmt.Errorf("failed to configure payments service: %w", err)
					}
					break
				}
				// no - poll for operator shares until we can attempt to decrypt the file
				<-time.After(60 * time.Second) // wait a minute before attempting again to get operator shares
			}
			return nil, nil, nil
		}()
		if err != nil {
			logger.Error().Err(err).Msg("something went wrong during vault unseal")
		}
	}()

	if err := service.configureDatastore(ctx); err != nil {
		logger.Fatal().Err(err).Msg("could not configure datastore")
		return nil, nil, errors.New("could not configure datastore")
	}

	return ctx, service, nil
}

// DecryptBootstrap - use service keyShares to reconstruct the decryption key.
func (s *Service) DecryptBootstrap(
	ctx context.Context,
	ciphertext []byte,
) (map[string]string, error) {
	// combine the service configured key shares
	key, err := shamir.Combine(s.keyShares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine keyShares: %w", err)
	}

	// pull nonce off ciphertext blob
	var nonce [32]byte
	copy(nonce[:], ciphertext[:32]) // nonce is in first 32 bytes of ciphertext

	// shove key into array
	var k [32]byte
	copy(k[:], key) // nonce is in first 32 bytes of ciphertext

	// decrypted is the encryption key used to decrypt secrets now
	v, err := cryptography.DecryptMessage(k, ciphertext[32:], nonce[:])
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// then unmarshal the json configuration file and load the secrets in memory
	var output = map[string]string{}
	err = json.Unmarshal([]byte(v), &output)
	if err != nil {
		return nil, fmt.Errorf("failed to json decode secrets: %w", err)
	}

	return output, nil
}

func isQLDBReady(ctx context.Context) bool {
	logger := logging.Logger(ctx, "payments.isQLDBReady")
	// decrypt the aws region
	qldbArn, qldbArnOK := ctx.Value(appctx.PaymentsQLDBRoleArnCTXKey).(string)
	// decrypt the aws region
	region, regionOK := ctx.Value(appctx.AWSRegionCTXKey).(string)
	// get proxy address for outbound
	egressAddr, egressOK := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if regionOK && egressOK && qldbArnOK {
		return true
	}
	logger.Warn().
		Str("region", region).
		Str("egressAddr", egressAddr).
		Str("qldbArn", qldbArn).
		Msg("service is not configured to access qldb")
	return false
}
