package service

import (
	"context"

	"github.com/brave-intl/bat-go/utils/clients/ledger"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

// Service contains datastore and ledger client connections
type Service struct {
	Datastore    Datastore
	RoDatastore  ReadOnlyDatastore
	LedgerClient ledger.Client
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	ledgerClient, err := ledger.New()
	if err != nil {
		return nil, err
	}

	service := &Service{
		Datastore:    datastore,
		RoDatastore:  roDatastore,
		LedgerClient: ledgerClient,
	}
	return service, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (service *Service) ReadableDatastore() ReadOnlyDatastore {
	if service.RoDatastore != nil {
		return service.RoDatastore
	}
	return service.Datastore
}

// GetOrCreateWallet attempts to retrieve wallet info from the local datastore, falling back to the ledger
func (service *Service) GetOrCreateWallet(ctx context.Context, walletID uuid.UUID) (*wallet.Info, error) {
	wallet, err := service.ReadableDatastore().GetWallet(walletID)
	if err != nil {
		return nil, errors.Wrap(err, "Error looking up wallet")
	}

	if wallet == nil {
		wallet, err = service.LedgerClient.GetWallet(ctx, walletID)
		if err != nil {
			return nil, errors.Wrap(err, "Error looking up wallet")
		}
		if wallet != nil {
			err = service.Datastore.InsertWallet(wallet)
			if err != nil {
				return nil, errors.Wrap(err, "Error saving wallet")
			}
		}
	}
	return wallet, nil
}

// SubmitAnonCardTransaction validates and submits a transaction on behalf of an anonymous card
func (service *Service) SubmitAnonCardTransaction(ctx context.Context, walletID uuid.UUID, transaction string) (*wallet.TransactionInfo, error) {
	walletInfo, err := service.GetOrCreateWallet(ctx, walletID)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting wallet")
	}
	providerWallet, err := provider.GetWallet(*walletInfo)
	if err != nil {
		return nil, err
	}
	anonCard, ok := providerWallet.(*uphold.Wallet)
	if !ok {
		return nil, errors.New("Only uphold wallets are supported")
	}

	// FIXME needs to require the idempotency key
	_, err = anonCard.VerifyAnonCardTransaction(transaction)
	if err != nil {
		return nil, err
	}

	// Submit and confirm since we are requiring the idempotency key
	return anonCard.SubmitTransaction(transaction, true)
}
