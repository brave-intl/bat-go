package payments

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/libs/httpsignature"
)

// the list of payment verifiers, mapping to individuals in payments-ops
var validVerifiers = map[string]bool{
	"2b2ddfcfba5045fac57efaf9c6a21e61a0bd7eee3c75e4ad1ee159c7e83cee43": true,
	"7f5fd7dab95cf7e4925651e18fb71b4e64b23734736f6834f3d633a44fd371d8": true,
	"b1db87ca0754dd9ade0246e4029d955be4e0a50f7a97026938d104162e1e0204": true,
}

// ErrInvalidVerifier - error stating the keyID is not a valid verifier
var ErrInvalidVerifier = errors.New("not a valid verifier")

// LookupVerifier - implement keystore for httpsignature
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {

	// keyID is the public key, we need to see if this exists in our verifier map
	if allowed, exists := validVerifiers[keyID]; !exists || !allowed {
		return nil, nil, ErrInvalidVerifier
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
