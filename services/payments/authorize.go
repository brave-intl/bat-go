package payments

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// validAuthorizers is the list of payment authorizers, mapping to individuals in payments-ops.
var validAuthorizers = map[string]bool{
	// test/private.pem
	"a5700b95f77fa0fc078cd923ad5075a100d6b995ecc86e49919a0f6ee45ee983": true,
	// test/private2.pem
	"732afdb29da6d5ab8481b247d9b2724d79c3652dddc64eb5ad251a2679e6210d": true,
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
		_, err := writeTransaction(
			ctx,
			s.datastore,
			s.sdkClient,
			s.kmsSigningClient,
			s.kmsSigningKeyID,
			transaction,
		)
		if err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}
	}
	return nil
}

// DriveTransaction attempts to Drive the Transaction forward.
func (s *Service) DriveTransaction(
	ctx context.Context,
	transaction *paymentLib.AuthenticatedPaymentState,
) error {
	stateMachine, err := StateMachineFromTransaction(s, transaction)
	if err != nil {
		return fmt.Errorf("failed to create stateMachine: %w", err)
	}

	transaction, err = Drive(ctx, stateMachine)
	if err != nil {
		// Insufficient authorizations is an expected state. Treat it as such.
		var insufficientAuthorizations *InsufficientAuthorizationsError
		if errors.As(err, &insufficientAuthorizations) {
			return nil
		}
		return fmt.Errorf("failed to progress transaction: %w", err)
	}
	_, err = writeTransaction(
		ctx,
		s.datastore,
		s.sdkClient,
		s.kmsSigningClient,
		s.kmsSigningKeyID,
		transaction,
	)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}
	return nil
}
