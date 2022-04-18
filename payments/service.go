package payments

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/logging"
	appsrv "github.com/brave-intl/bat-go/utils/service"
)

// Service - struct definition of payments service
type Service struct {
	baseCtx   context.Context
	secretMgr appsrv.SecretManager
	pubKey    *[32]byte
	privKey   *[32]byte
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
func (s *Service) decryptSecrets(ctx context.Context, secrets []byte, ciphertextB64 string, senderKeyHex string) (map[appctx.CTXKey]interface{}, error) {
	// ciphertext is the nacl box encrypted short shared key for decrypting secrets
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
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
	copy(decryptNonce[:], ciphertext[:24])

	var senderKeyT [32]byte
	copy(senderKeyT[:], senderKey[:32])

	var privKey [32]byte
	copy(privKey[:], s.privKey[:32])

	key, ok := box.Open(nil, ciphertext[24:], &decryptNonce, &senderKeyT, &privKey)
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
