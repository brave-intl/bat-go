package promotion

import (
	"context"

	"github.com/brave-intl/bat-go/wallet"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

// GetOrCreateWallet attempts to retrieve wallet info from the local datastore, falling back to the ledger
func (service *Service) GetOrCreateWallet(ctx context.Context, walletID uuid.UUID) (*wallet.Info, error) {
	wallet, err := service.ReadableDatastore().GetWallet(walletID)
	if err != nil {
		return nil, errors.Wrap(err, "Error looking up wallet")
	}

	if wallet == nil {
		wallet, err = service.ledgerClient.GetWallet(ctx, walletID)
		if err != nil {
			return nil, errors.Wrap(err, "Error looking up wallet")
		}
		if wallet != nil {
			err = service.datastore.InsertWallet(wallet)
			if err != nil {
				return nil, errors.Wrap(err, "Error saving wallet")
			}
		}
	}
	return wallet, nil
}
