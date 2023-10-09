package payments

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	nitro_eclave_attestation_document "github.com/veracruz-project/go-nitro-enclave-attestation-document"

	"encoding/hex"
	"encoding/pem"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/nitro"

	appctx "github.com/brave-intl/bat-go/libs/context"
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

	baseCtx           context.Context
	secretMgr         appsrv.SecretManager
	keyShares         [][]byte
	secretsCiphertext []byte
	secrets           map[string]string
	kmsDecryptKeyArn  string
	sdkClient         wrappedQldbSDKClient

	publicKey []byte
	signer    paymentLib.Signator
	verifier  paymentLib.Verifier
}

var (
	errNoSecretsBucketConfigured        = errors.New("no secrets bucket name for payments service")
	errNoSecretsObjectConfigured        = errors.New("no secrets object name for payments service")
	errNoOperatorSharesBucketConfigured = errors.New("no secrets operator shares bucket name for payments service")
)

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

	// create the kms encryption key for this service for bootstrap operator shares
	if err := service.configureKMSEncryptionKey(ctx); err != nil {
		return nil, nil, fmt.Errorf("could not create kms secret encryption key: %w", err)
	}

	go func() {
		_, _, err := func() (interface{}, interface{}, error) {
			// get the secrets object key and bucket name from environment
			secretsBucketName, ok := ctx.Value(appctx.EnclaveSecretsBucketNameCTXKey).(string)
			if !ok {
				return nil, nil, errNoSecretsBucketConfigured
			}

			// download the configuration file, kms decrypt the file
			secretsObjectName, ok := ctx.Value(appctx.EnclaveSecretsObjectNameCTXKey).(string)
			if !ok {
				return nil, nil, errNoSecretsObjectConfigured
			}

			for {
				// fetch the secrets, result will store the secrets (age ciphertext) on the service instance
				if err := service.fetchSecrets(ctx, secretsBucketName, secretsObjectName); err != nil {
					// log the error, we will retry again
					logger.Error().Err(err).Msg("failed to fetch secrets, will retry shortly")
					<-time.After(30 * time.Second)
					continue
				}
				break
			}

			// operator shares files
			operatorSharesBucketName, ok := ctx.Value(appctx.EnclaveOperatorSharesBucketNameCTXKey).(string)
			if !ok {
				return nil, nil, errNoOperatorSharesBucketConfigured
			}

			for {
				// do we have enough shares to attempt to reconstitute the key?
				if err := service.fetchOperatorShares(ctx, operatorSharesBucketName); err != nil {
					logger.Error().Err(err).Msg("failed to fetch operator shares, will retry shortly")
					<-time.After(60 * time.Second)
					continue
				}
				if ok := service.enoughOperatorShares(ctx, 2); ok { // 2 is the number of shares required
					// yes - attempt to decrypt the file
					if err := service.configureSecrets(ctx); err != nil {
						// fail to decrypt?  panic loudly
						return nil, nil, fmt.Errorf("failed to configure payments service secrets: %w", err)
					}
					break
				}
				logger.Error().Err(err).Msg("need more operator shares to decrypt secrets")
				// no - poll for operator shares until we can attempt to decrypt the file
				<-time.After(60 * time.Second) // wait a minute before attempting again to get operator shares
			}
			// at this point we should have our config loaded, lets print out the keys
			for k := range service.secrets {
				logger.Info().Msgf("%s is loaded in secrets!", k)
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
