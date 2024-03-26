package payments

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"golang.org/x/crypto/ssh"
)

var validAuthorizerKeys = map[string][]string{
	"production": {},
	"staging": {
		// @evq
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA91/jZI+hcisdAURdqgdAKyetA4b2mVJIypfEtTyXW+ evq+settlements@brave.com",
		// @sneagan
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDfcr9jUEu9D9lSpUnPwT1cCggCe48kZw1bJt+CXYSnh jegan+settlements@brave.com",
		// @jtieman
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIK1fxpURIUAJNRqosAnPPXnKjpUBGGOKgkUOXmviJfFx jtieman+nitro@brave.com",
	},
	"development": {
		// @kdenhartog for dev environment only
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEY/3VGKsrH5dp3mK5PJIHVkUMWpsmUhZkrLuZTf7Sqr kdenhartog+settlement+dev@brave.com",
		// two development keys
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINGylZXIukc6tYnLj6wuSlg/foMCnslAEwFl7qG+TuBK dev1+settlements@brave.com",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINBBEASr19T3JQ1U7SFO2EcZDfqYjUkBlBtVq+KLQtmY dev2+settlements@brave.com",
	},
}

// validAuthorizers is the list of payment authorizers, mapping to individuals in payments-ops.
var validAuthorizers = make(map[string]httpsignature.Ed25519PubKey)

func init() {
	for _, key := range validAuthorizerKeys[os.Getenv("ENV")] {
		pub, err := DecodePublicKey(key)
		if err != nil {
			panic(err)
		}
		validAuthorizers[hex.EncodeToString(pub)] = pub
	}
	fmt.Println(validAuthorizers)
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
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	// keyID is the public key, we need to see if this exists in our verifier map
	publicKey, exists := validAuthorizers[keyID]
	if !exists {
		return nil, nil, &ErrInvalidAuthorizer{}
	}

	verifier := httpsignature.Verifier(publicKey)
	return ctx, &verifier, nil
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
