package payments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"text/template"

	"encoding/hex"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/rs/zerolog"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// Service struct definition of payments service.
type Service struct {
	// concurrent safe
	datastore  Datastore
	awsCfg     aws.Config
	egressAddr string

	baseCtx          context.Context
	kmsDecryptKeyArn string
	operatorKey      OperatorKey
	secretsLoaded    atomic.Bool

	publicKey     string
	signer        paymentLib.Signator
	verifierStore paymentLib.Keystore
}

var (
	errNoSecretsBucketConfigured        = errors.New("no secrets bucket name for payments service")
	errNoSecretsObjectConfigured        = errors.New("no secrets object name for payments service")
	errNoSolanaAddressConfigured        = errors.New("no solana address for payments service")
	errNoOperatorSharesBucketConfigured = errors.New("no secrets operator shares bucket name for payments service")
)

func parseKeyPolicyTemplate(ctx context.Context, templateFile string) (string, string, error) {
	var logger = logging.Logger(ctx, "payments.parseKeyPolicyTemplate")

	pcrs, err := nitro.GetPCRs()
	if err != nil {
		return "", "", fmt.Errorf("failed to get PCR values: %w", err)
	}
	logger.Info().Msgf("pcrs: %+v", pcrs)

	t, err := template.ParseFiles(templateFile)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template file: %+v", templateFile)
		return "", "", err
	}

	type keyTemplateData struct {
		PCR0                     string
		PCR1                     string
		PCR2                     string
		AWSAccount               string
		NodeGroupRole            string
		SettlementsDeveloperRole string
	}

	buf := bytes.NewBuffer([]byte{})
	if err := t.Execute(buf, keyTemplateData{
		PCR0:                     hex.EncodeToString(pcrs[0]),
		PCR1:                     hex.EncodeToString(pcrs[1]),
		PCR2:                     hex.EncodeToString(pcrs[2]),
		AWSAccount:               os.Getenv("AWS_ACCOUNT"),
		NodeGroupRole:            os.Getenv("NODE_GROUP_ROLE"),
		SettlementsDeveloperRole: os.Getenv("SETTLEMENTS_DEVELOPER_ROLE"),
	}); err != nil {
		logger.Error().Err(err).Msgf("failed to execute template file: %+v", templateFile)
		return "", "", err
	}

	policy := buf.String()

	logger.Info().Msgf("key policy: %+v", policy)

	return policy, hex.EncodeToString(pcrs[2]), nil
}

// configureKMSEncryptionKey creates the enclave kms key which is only decrypt capable with enclave
// attestation.
func (s *Service) configureKMSEncryptionKey(ctx context.Context) error {
	// get the aws configuration loaded
	cfg := s.awsCfg
	kmsClient := kms.NewFromConfig(cfg)
	var logger = logging.Logger(ctx, "payments.ConfigureKMSEncryptionKey")

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
		logger.Info().Msgf("%+v - getKeyResult!", getKeyResult)
		// key exists, check the key policy matches
		// if the key alias already exists, pull down that particular key policy
		listKeyPolicyResult, err := kmsClient.ListKeyPolicies(ctx, &kms.ListKeyPoliciesInput{
			KeyId: getKeyResult.KeyMetadata.KeyId,
		})
		if err != nil {
			return fmt.Errorf("failed to list key for alias: %w", err)
		}

		// this should always be one policy, named `default`
		// aws says we should get the list of names though, who am i to argue
		if listKeyPolicyResult == nil || len(listKeyPolicyResult.PolicyNames) != 1 {
			return fmt.Errorf("wrong number of key policies for alias")
		}

		// actually pull the key
		getKeyPolicyResult, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId:      getKeyResult.KeyMetadata.KeyId,
			PolicyName: &listKeyPolicyResult.PolicyNames[0],
		})
		if err != nil {
			return fmt.Errorf("failed to get key policy for alias: %w", err)
		}

		// we will now check that the retrieved policy matches the policy we just
		// created, and if so we will use this kms key in the enclave
		var generated, retrieved interface{}
		if err := json.Unmarshal([]byte(*getKeyPolicyResult.Policy), &retrieved); err != nil {
			return fmt.Errorf("invalid json returned from get key policy result: %w", err)
		}
		if err := json.Unmarshal([]byte(policy), &generated); err != nil {
			return fmt.Errorf("invalid json generated from key policy template: %w", err)
		}

		// aws policy will alter the whitespace in the policy
		if reflect.DeepEqual(generated, retrieved) {
			logger.Info().Msgf("policy matches: \n %s \n %s!", *getKeyPolicyResult.Policy, policy)
			// if the policy matches, we should use this key
			s.kmsDecryptKeyArn = *getKeyResult.KeyMetadata.KeyId
			return nil
		}
		logger.Info().Msgf("policy does not match: \n\n %s \n\n %s!", *getKeyPolicyResult.Policy, policy)
		panic("failed to match policy text")
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

	logger.Info().Msgf("created new key! %+v", input)

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String("alias/decryption-" + imageSHA),
		TargetKeyId: result.KeyMetadata.KeyId,
	}

	_, err = kmsClient.CreateAlias(ctx, aliasInput)
	if err != nil {
		var aee *kmsTypes.AlreadyExistsException
		if !errors.As(err, &aee) {
			return fmt.Errorf("failed to make key alias: %w", err)
		}
		logger.Info().Msgf("alias already exists! %+v", err)
	}
	logger.Info().Msgf("created new alias! %+v", input)

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
	store, err := NewVerifierStore()
	if err != nil {
		logger.Fatal().Err(err).Msg("could not create verifier store")
		return nil, nil, errors.New("could not create verifier store")
	}

	service := &Service{
		baseCtx:       ctx,
		awsCfg:        awsCfg,
		publicKey:     hex.EncodeToString(pcrs[2]),
		signer:        nitro.Signer{},
		verifierStore: store,
		egressAddr:    egressAddr,
	}

	// create the kms encryption key for this service for bootstrap operator shares
	if err := service.configureKMSEncryptionKey(ctx); err != nil {
		return nil, nil, fmt.Errorf("could not create kms secret encryption key: %w", err)
	}

	service.datastore, err = configureDatastore(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("could not configure datastore")
		return nil, nil, errors.New("could not configure datastore")
	}

	// Start Vault unseal in the background.
	go func() {
		err := service.unsealConfig(ctx, logger)
		if err != nil {
			logger.Error().Err(err).Msg("something went wrong during vault unseal")
		}
	}()

	return ctx, service, nil
}

func (s *Service) unsealConfig(
	ctx context.Context,
	logger *zerolog.Logger,
) error {
	unsealing := &Unsealing{
		kmsDecryptKeyArn: s.kmsDecryptKeyArn,
		getChainAddress:  s.datastore.GetChainAddress,
	}

	operatorErrorCh := make(chan error, 1)
	go func() {
		operatorErrorCh <- unsealing.fetchOperatorShares(ctx, logger)
	}()

	err := unsealing.fetchSecretes(ctx, logger)
	err2 := <-operatorErrorCh
	if err != nil || err2 != nil {
		return errors.Join(err, err2)
	}

	err = unsealing.decryptSecrets(ctx)
	if err != nil {
		return err
	}

	logger.Debug().Msg("decrypted secrets without error")

	// at this point we should have our config loaded, lets print out what keys
	// we got
	for k := range unsealing.secrets {
		logger.Info().Msgf("%s is loaded in secrets!", k)
	}

	s.setEnvFromSecrets(ctx, unsealing.secrets)
	logger.Debug().Msg("set env from secrets")

	s.operatorKey = unsealing.operatorKey
	s.secretsLoaded.Store(true)
	return nil
}

// AreSecretsLoaded will tell you if we have successfully loaded secrets on the service
func (s *Service) AreSecretsLoaded(ctx context.Context) bool {
	return s.secretsLoaded.Load()
}

// setEnvFromSecrets takes a secrets map and loads the secrets as environment variables
func (s *Service) setEnvFromSecrets(ctx context.Context, secrets map[string]string) {
	logger := logging.Logger(ctx, "payments.secrets")
	os.Setenv("ZEBPAY_API_KEY", secrets["zebpayApiKey"])
	os.Setenv("ZEBPAY_SIGNING_KEY", secrets["zebpayPrivateKey"])
	os.Setenv("SOLANA_RPC_ENDPOINT", secrets["solanaRpcEndpoint"])

	if solKey, ok := secrets["solanaPrivateKey"]; ok {
		logger.Debug().Int("solana key length", len(secrets["solanaPrivateKey"])).Msg("setting solana key environment varialbe")
		os.Setenv("SOLANA_SIGNING_KEY", solKey)
		logger.Debug().Int("solana env var key length", len(os.Getenv("SOLANA_SIGNING_KEY"))).Msg("set solana key environment varialbe")
	}
}
