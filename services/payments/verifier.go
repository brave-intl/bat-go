package payments

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/brave-intl/bat-go/libs/httpsignature"
)

// validVerifiers is the list of payment verifiers, mapping to individuals in payments-ops.
var validVerifiers = map[string]bool{
	// test/private.pem
	"a5700b95f77fa0fc078cd923ad5075a100d6b995ecc86e49919a0f6ee45ee983": true,
	// test/private2.pem
	"732afdb29da6d5ab8481b247d9b2724d79c3652dddc64eb5ad251a2679e6210d": true,
}

// LookupVerifier implements keystore for httpsignature.
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	// keyID is the public key, we need to see if this exists in our verifier map
	if allowed, exists := validVerifiers[keyID]; !exists || !allowed {
		return nil, nil, &ErrInvalidVerifier{}
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
