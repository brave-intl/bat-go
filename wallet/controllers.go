package wallet

import (
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	uuid "github.com/satori/go.uuid"
)

// LookupPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, errorutils.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.GetWallet(walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}

	if wallet == nil {
		return nil, nil
	}

	var publicKey httpsignature.Ed25519PubKey
	if len(wallet.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(wallet.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	tmp := httpsignature.Verifier(publicKey)
	return &tmp, nil
}

func validateHTTPSignature(ctx context.Context, r *http.Request, signature string) (string, error) {
	// validate that the signature in the header is valid based on public key provided
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(signature))
	if err != nil {
		return "", fmt.Errorf("invalid signature: %w", err)
	}

	// Override algorithm and headers to those we want to enforce
	s.Algorithm = httpsignature.ED25519
	s.Headers = []string{"digest", "(request-target)"}
	var publicKey httpsignature.Ed25519PubKey
	if len(s.KeyID) > 0 {
		var err error
		publicKey, err = hex.DecodeString(s.KeyID)
		if err != nil {
			return "", fmt.Errorf("failed to hex decode public key: %w", err)
		}
	} else {
		// there was no KeyId in the Signature
		if err != nil {
			return "", errors.New("no KeyId found in the HTTP Signature")
		}
	}
	pubKey := httpsignature.Verifier(publicKey)
	if err != nil {
		return "", err
	}
	if pubKey == nil {
		return "", errors.New("invalid public key")
	}

	valid, err := s.Verify(pubKey, crypto.Hash(0), r)

	if err != nil {
		return "", fmt.Errorf("failed to verify signature: %w", err)
	}
	if !valid {
		return "", errors.New("invalid signature")
	}
	return s.KeyID, nil
}
