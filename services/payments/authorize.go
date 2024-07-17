package payments

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"golang.org/x/crypto/ssh"
)

type Authorizers struct {
	keys map[string]httpsignature.Ed25519PubKey
}

var managementAuthorizers Authorizers

var paymentAuthorizers Authorizers

func init() {
	if err := managementAuthorizers.AddKeys(vaultManagerKeys()); err != nil {
		panic(err)
	}

	if err := paymentAuthorizers.AddKeys(paymentOperatorKeys()); err != nil {
		panic(err)
	}

	// paymentAuthorizers includes the managers
	for k, v := range managementAuthorizers.keys {
		paymentAuthorizers.keys[k] = v
	}
}

func (a *Authorizers) AddKeys(authorizedPublicKeys []string) error {
	if a.keys == nil {
		a.keys = make(map[string]httpsignature.Ed25519PubKey)
	}
	for _, key := range authorizedPublicKeys {
		pub, err := DecodePublicKey(key)
		if err != nil {
			return fmt.Errorf("invalid authorizer public key - %w", err)
		}
		a.keys[hex.EncodeToString(pub)] = pub
	}
	return nil
}

// DecodePublicKey decodes the public key which can either be a raw hex encoded ed25519 public key or an ssh-ed25519 public key
func DecodePublicKey(key string) (httpsignature.Ed25519PubKey, error) {
	if strings.HasPrefix(key, "ssh-ed25519") {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("failed to parse ssh public key: %w", err)
		}

		if pubKey.Type() != "ssh-ed25519" {
			return nil, fmt.Errorf("public key is not ed25519 public key")
		}
		cPubKey, ok := pubKey.(ssh.CryptoPublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not ed25519 public key")
		}
		edKey, ok := cPubKey.CryptoPublicKey().(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not ed25519 public key")
		}
		return httpsignature.Ed25519PubKey(edKey), nil
	}
	var (
		edKey httpsignature.Ed25519PubKey
		err   error
	)

	edKey, err = hex.DecodeString(key)
	return edKey, err
}

// LookupVerifier implements keystore for httpsignature.
func (a *Authorizers) LookupVerifier(
	ctx context.Context,
	keyID string,
) (context.Context, httpsignature.Verifier, error) {
	// keyID is the public key, we need to see if this exists in our verifier map
	publicKey, exists := a.keys[keyID]
	if !exists {
		return nil, nil, &ErrInvalidAuthorizer{}
	}

	return ctx, publicKey, nil
}

// AuthorizeTransaction - Add an Authorization for the Transaction
// NOTE: This function assumes that the http signature has been
// verified before running. This is achieved in the SubmitHandler middleware.
func (s *Service) AuthorizeTransaction(
	ctx context.Context,
	keyID string,
	transaction *paymentLib.AuthenticatedPaymentState,
) error {
	auth := paymentLib.PaymentAuthorization{
		KeyID:      keyID,
		DocumentID: transaction.DocumentID,
	}
	keyHasNotYetSigned := true
	for _, authorization := range transaction.Authorizations {
		if authorization.KeyID == auth.KeyID {
			keyHasNotYetSigned = false
		}
	}
	if keyHasNotYetSigned {
		transaction.Authorizations = append(transaction.Authorizations, auth)
	}
	return nil
}
