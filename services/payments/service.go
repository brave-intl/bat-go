package payments

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"

	"encoding/hex"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/brave-intl/bat-go/libs/nitro"

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

	baseCtx              context.Context
	keyShares            [][]byte
	secretsCiphertext    []byte
	solanaPrivCiphertext []byte
	secrets              map[string]string
	kmsDecryptKeyArn     string

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
			solanaAddress, ok := ctx.Value(appctx.EnclaveSolanaAddressCTXKey).(string)
			if !ok {
				return nil, nil, errNoSolanaAddressConfigured
			}
			logger.Debug().Str("solana address:", solanaAddress).Msg("solana address configured")

			for {
				// fetch the secrets, result will store the secrets (age ciphertext) on the service instance
				if err := service.fetchSecrets(ctx, secretsBucketName, secretsObjectName, solanaAddress); err != nil {
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
				// yes - attempt to decrypt the file
				if err := service.configureSecrets(ctx); err != nil {
					// fail to decrypt?  panic loudly
					return nil, nil, fmt.Errorf("failed to configure payments service secrets: %w", err)
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
