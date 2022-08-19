package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	uuid "github.com/satori/go.uuid"
)

// LookupVerifier based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.GetWallet(ctx, walletID)
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "error getting wallet")
	}

	if wallet == nil {
		return nil, nil, nil
	}

	var publicKey httpsignature.Ed25519PubKey
	if len(wallet.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(wallet.PublicKey)
		if err != nil {
			return nil, nil, err
		}
	}
	tmp := httpsignature.Verifier(publicKey)
	return ctx, &tmp, nil
}

// DecodeEd25519Keystore is a keystore that "looks up" a verifier by attempting to decode the keyID as a base64 encoded ed25519 public key
type DecodeEd25519Keystore struct{}

// LookupVerifier by decoding keyID
func (d *DecodeEd25519Keystore) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	var publicKey httpsignature.Ed25519PubKey
	if len(keyID) > 0 {
		var err error
		publicKey, err = hex.DecodeString(keyID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to hex decode public key: %w", err)
		}
	} else {
		return nil, nil, errors.New("empty KeyId is not valid")
	}
	verifier := httpsignature.Verifier(publicKey)
	return ctx, &verifier, nil
}
