package payments

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// validAuthorizers is the list of payment authorizers, mapping to individuals in payments-ops.
var validAuthorizers = map[string]bool{
	// @evq
	"7cc23f59ff7055fe6d0aa2fc04e024691a7f347898ff366bcbc83c1e622e62ec": true,
	// @sneagan
	"1dadd1382d26dd10442b7981d18334327deb143fcd19ff1c98ed38fbf3ca5d8a": true
}

// LookupVerifier implements keystore for httpsignature.
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	// keyID is the public key, we need to see if this exists in our verifier map
	if allowed, exists := validAuthorizers[keyID]; !exists || !allowed {
		return nil, nil, &ErrInvalidAuthorizer{}
	}

	var (
		publicKey httpsignature.Ed25519PubKey
		err       error
	)

	publicKey, err = hex.DecodeString(keyID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode verifier public key: %w", err)
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
