package payments

import (
	"context"
	"fmt"

	"github.com/awslabs/amazon-qldb-driver-go/v2/qldbdriver"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/custodian"

	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"golang.org/x/crypto/nacl/box"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/logging"
	appsrv "github.com/brave-intl/bat-go/utils/service"
)

// Service - struct definition of payments service
type Service struct {
	// concurrent safe
	datastore              *qldbdriver.QLDBDriver
	processTransaction     chan Transaction
	stopProcessTransaction func()
	custodians             map[string]custodian.Custodian

	baseCtx   context.Context
	secretMgr appsrv.SecretManager
	pubKey    *[32]byte
	privKey   *[32]byte
}

// NewService creates a service using the passed datastore and clients configured from the environment
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var logger = logging.Logger(ctx, "payments.NewService")

	driver, err := newQLDBDatastore(ctx)

	if err != nil {
		logger.Fatal().Err(err).Msg("failed to setup qldb")
	}

	// custodian transaction processing channel and stop signal
	// buffer up to 25000 transactions for processing at a time
	processTransaction := make(chan Transaction, 25000)
	ctx, stopProcessTransaction := context.WithCancel(ctx)

	// setup our custodian integrations
	upholdCustodian, err := custodian.New(ctx, custodian.Config{Provider: custodian.Uphold})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create uphold custodian")
		defer stopProcessTransaction()
		return ctx, nil, fmt.Errorf("failed to create uphold custodian: %w", err)
	}
	geminiCustodian, err := custodian.New(ctx, custodian.Config{Provider: custodian.Gemini})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create gemini custodian")
		defer stopProcessTransaction()
		return ctx, nil, fmt.Errorf("failed to create gemini custodian: %w", err)
	}
	bitflyerCustodian, err := custodian.New(ctx, custodian.Config{Provider: custodian.Bitflyer})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create bitflyer custodian")
		defer stopProcessTransaction()
		return ctx, nil, fmt.Errorf("failed to create bitflyer custodian: %w", err)
	}

	service := &Service{
		// initialize qldb datastore
		datastore:              driver,
		processTransaction:     processTransaction,
		stopProcessTransaction: stopProcessTransaction,
		custodians: map[string]custodian.Custodian{
			custodian.Uphold:   upholdCustodian,
			custodian.Gemini:   geminiCustodian,
			custodian.Bitflyer: bitflyerCustodian,
		},
	}

	// startup our transaction processing job
	go func() {
		if err := service.ProcessTransactions(ctx); err != nil {
			logger.Fatal().Err(err).Msg("failed to setup transaction processing job")
		}
	}()

	return ctx, service, nil
}

// ProcessTransactions - read transactions off a channel and process them with custodian
func (s *Service) ProcessTransactions(ctx context.Context) error {
	var logger = logging.Logger(ctx, "payments.ProcessTransactions")

	for {
		select {
		case <-ctx.Done():
			logger.Warn().Msg("context cancelled, no longer processing transactions")
			return nil
		case transaction := <-s.processTransaction:
			logger.Debug().Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("processing a transaction")
			// create a custodian transaction from this transaction:
			custodianTransaction, err := custodian.NewTransaction(
				ctx, transaction.IdempotencyKey, transaction.To, transaction.From, altcurrency.BAT, transaction.Amount,
			)

			if err != nil {
				logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("could not create custodian transaction")
				continue
			}

			if c, ok := s.custodians[transaction.Custodian]; ok {
				// TODO: store the full response from submit transaction
				err = c.SubmitTransactions(ctx, custodianTransaction)
				if err != nil {
					logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("failed to submit transaction")
					continue
				}
			} else {
				logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("invalid custodian")
				continue
			}
		}
	}
}

// initialize the service
func initService(ctx context.Context) (*Service, error) {
	logger := logging.Logger(ctx, "initService")
	// generate the ed25519 pub/priv keypair for secrets management
	pubKey, privKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		logger.Error().Err(err).Msg("failed to generate keypair")
		return nil, fmt.Errorf("failed to initialize service: %w", err)
	}
	return &Service{
		baseCtx:   ctx,
		secretMgr: &awsClient{},
		// keys used for encryption/decryption of configuration secrets
		pubKey:  pubKey,
		privKey: privKey,
	}, nil
}

// decryptSecrets - perform nacl box to get the configuration encryption key from exchange
func (s *Service) decryptSecrets(ctx context.Context, secrets []byte, keyCiphertextB64 string, senderKeyHex string) (map[appctx.CTXKey]interface{}, error) {
	// ciphertext is the nacl box encrypted short shared key for decrypting secrets
	keyCiphertext, err := base64.StdEncoding.DecodeString(keyCiphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to b64 decode ciphertext: %w", err)
	}

	// sender key is the ephemeral sender public key for nacl box
	senderKey, err := hex.DecodeString(senderKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to hex decode sender key: %w", err)
	}

	// get nonce from ciphertext, keeping it in first 24 bytes
	var decryptNonce [24]byte
	copy(decryptNonce[:], keyCiphertext[:24])

	var senderKeyT [32]byte
	copy(senderKeyT[:], senderKey[:32])

	var privKey [32]byte
	copy(privKey[:], s.privKey[:32])

	key, ok := box.Open(nil, keyCiphertext[24:], &decryptNonce, &senderKeyT, &privKey)
	if !ok {
		return nil, errors.New("decryption error")
	}

	var decryptKey [32]byte
	copy(decryptKey[:], key[:32])

	// decrypted is the encryption key used to decrypt secrets now
	v, err := cryptography.DecryptMessage(decryptKey, secrets[24:], secrets[:24])
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
