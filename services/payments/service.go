package payments

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/hashicorp/vault/shamir"

	"encoding/json"

	awsutils "github.com/brave-intl/bat-go/libs/aws"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	appsrv "github.com/brave-intl/bat-go/libs/service"
)

// Service - struct definition of payments service
type Service struct {
	// concurrent safe
	datastore  *qldbdriver.QLDBDriver
	custodians map[string]provider.Custodian
	awsCfg     aws.Config

	baseCtx          context.Context
	secretMgr        appsrv.SecretManager
	keyShares        [][]byte
	kmsDecryptKeyArn string
}

// createKMSKey creates the enclave kms key which is only decrypt capable with enclave attestation.
func (s *Service) configureKMSKey(ctx context.Context) error {
	// perform enclave attestation
	nonce := make([]byte, 64)
	_, err := rand.Read(nonce)
	if err != nil {
		return fmt.Errorf("failed to create nonce for attestation: %w", err)
	}
	document, err := nitro.Attest(nonce, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create attestation document: %w", err)
	}
	var logger = logging.Logger(ctx, "payments.configureKMSKey")
	logger.Debug().Msgf("document: %+v", document)

	// get the aws configuration loaded
	cfg := s.awsCfg

	// TODO: get the pcr values for the condition from the document ^^
	imageSha384 := ""
	pcr0 := ""
	pcr1 := ""
	pcr2 := ""

	// get the secretsmanager id from ctx for the template
	templateSecretID, ok := ctx.Value(appctx.EnclaveDecryptKeyTemplateSecretIDCTXKey).(string)
	if !ok {
		return errors.New("template secret id for enclave decrypt key not found on context")
	}

	smClient := secretsmanager.NewFromConfig(cfg)
	o, err := smClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(templateSecretID),
	})

	if err != nil {
		return fmt.Errorf("failed to get key policy template from secrets manager: %w", err)
	}

	if o.SecretString == nil {
		return errors.New("secret is not defined in secrets manager")
	}

	keyPolicy := *o.SecretString
	keyPolicy = strings.ReplaceAll(keyPolicy, "<IMAGE_SHA384>", imageSha384)
	keyPolicy = strings.ReplaceAll(keyPolicy, "<PCR0>", pcr0)
	keyPolicy = strings.ReplaceAll(keyPolicy, "<PCR1>", pcr1)
	keyPolicy = strings.ReplaceAll(keyPolicy, "<PCR2>", pcr2)

	client := kms.NewFromConfig(cfg)

	input := &kms.CreateKeyInput{
		Policy: aws.String(keyPolicy),
	}

	result, err := awsutils.MakeKey(ctx, client, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	s.kmsDecryptKeyArn = *result.KeyMetadata.KeyId
	return nil
}

// NewService creates a service using the passed datastore and clients configured from the environment
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

	cfg, err := nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
	if err != nil {
		logger.Error().Msg("no egress addr for payments service")
		return nil, nil, errors.New("no egress addr for payments service")
	}

	service := &Service{
		baseCtx:   ctx,
		secretMgr: &awsClient{},
		awsCfg:    cfg,
	}

	if err := service.configureKMSKey(ctx); err != nil {
		// FIXME: handle create error better
		logger.Error().Err(err).Msg("could not create kms secret decryption key")
	}

	if err := service.configureDatastore(ctx); err != nil {
		logger.Fatal().Msg("could not configure datastore")
	}

	/*
			FIXME
		// setup our custodian integrations
		upholdCustodian, err := provider.New(ctx, provider.Config{Provider: provider.Uphold})
		if err != nil {
			logger.Error().Err(err).Msg("failed to create uphold custodian")
			return ctx, nil, fmt.Errorf("failed to create uphold custodian: %w", err)
		}
		geminiCustodian, err := provider.New(ctx, provider.Config{Provider: provider.Gemini})
		if err != nil {
			logger.Error().Err(err).Msg("failed to create gemini custodian")
			return ctx, nil, fmt.Errorf("failed to create gemini custodian: %w", err)
		}
		bitflyerCustodian, err := provider.New(ctx, provider.Config{Provider: provider.Bitflyer})
		if err != nil {
			logger.Error().Err(err).Msg("failed to create bitflyer custodian")
			return ctx, nil, fmt.Errorf("failed to create bitflyer custodian: %w", err)
		}

		service.custodians = map[string]provider.Custodian{
			provider.Uphold:   upholdCustodian,
			provider.Gemini:   geminiCustodian,
			provider.Bitflyer: bitflyerCustodian,
		}
	*/

	return ctx, service, nil
}

// DecryptBootstrap - use service keyShares to reconstruct the decryption key
func (s *Service) DecryptBootstrap(ctx context.Context, ciphertext []byte) (map[appctx.CTXKey]interface{}, error) {
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

	// decrypted message is a json blob, convert to our output
	var output = map[appctx.CTXKey]interface{}{}
	err = json.Unmarshal([]byte(v), &output)
	if err != nil {
		return nil, fmt.Errorf("failed to json decode secrets: %w", err)
	}

	return output, nil
}
