package payments

import (
	"context"
	"fmt"

	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/hashicorp/vault/shamir"

	"encoding/json"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	appsrv "github.com/brave-intl/bat-go/libs/service"
)

// Service - struct definition of payments service
type Service struct {
	// concurrent safe
	datastore  *qldbdriver.QLDBDriver
	custodians map[string]provider.Custodian

	baseCtx   context.Context
	secretMgr appsrv.SecretManager
	keyShares [][]byte
}

// NewService creates a service using the passed datastore and clients configured from the environment
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var logger = logging.Logger(ctx, "payments.NewService")

	service := &Service{
		baseCtx:   ctx,
		secretMgr: &awsClient{},
	}

	if err := service.configureDatastore(ctx); err != nil {
		logger.Fatal().Msg("could not configure datastore")
	}

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

	return ctx, service, nil
}

// decryptBootstrap - use service keyShares to reconstruct the decryption key
func (s *Service) decryptBootstrap(ctx context.Context, ciphertext []byte) (map[appctx.CTXKey]interface{}, error) {
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
