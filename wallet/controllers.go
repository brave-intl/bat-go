package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

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

	wallet, err := service.GetWallet(ctx, walletID)
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

type DecodeEd25519Keystore struct{}

func (d *DecodeEd25519Keystore) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	var publicKey httpsignature.Ed25519PubKey
	if len(keyID) > 0 {
		var err error
		publicKey, err = hex.DecodeString(keyID)
		if err != nil {
			return nil, fmt.Errorf("failed to hex decode public key: %w", err)
		}
	} else {
		return nil, errors.New("empty KeyId is not valid")
	}
	verifier := httpsignature.Verifier(publicKey)
	return &verifier, nil
}
