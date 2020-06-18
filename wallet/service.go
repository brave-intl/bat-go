package wallet

import (
	"context"
	"errors"

	"github.com/brave-intl/bat-go/utils/clients/ledger"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/wallet"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	uuid "github.com/satori/go.uuid"
)

var (
	walletClaimNamespace = uuid.Must(uuid.FromString("c39b298b-b625-42e9-a463-69c7726e5ddc"))
)

// Service contains datastore and ledger client connections
type Service struct {
	Datastore    Datastore
	RoDatastore  ReadOnlyDatastore
	LedgerClient ledger.Client
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context, datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	ledgerClient, err := ledger.NewFromContext(ctx)
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
		return nil, errorutils.Wrap(err, "error looking up wallet")
	}

	if wallet == nil {
		wallet, err = service.LedgerClient.GetWallet(ctx, walletID)
		if err != nil {
			return nil, errorutils.Wrap(err, "error looking up wallet")
		}
		if wallet != nil {
			err = service.Datastore.UpsertWallet(wallet)
			if err != nil {
				return nil, errorutils.Wrap(err, "error saving wallet")
			}
		}
	}
	return wallet, nil
}

// GetAndCreateMemberWallets all wallets associated with the wallet id provided are accounted for
func (service *Service) GetAndCreateMemberWallets(ctx context.Context, walletID uuid.UUID) (*wallet.Info, error) {
	// may be unnecessary to fetch
	wallets, err := service.LedgerClient.GetMemberWallets(ctx, walletID)
	if err != nil {
		return nil, err
	}
	if len(*wallets) == 0 {
		return nil, errorutils.ErrWalletNotFound
	}
	var info wallet.Info
	for _, w := range *wallets {
		err := service.Datastore.UpsertWallet(&w)
		if err != nil {
			return nil, err
		}
		if uuid.Equal(walletID, uuid.Must(uuid.FromString(w.ID))) {
			info = w
		}
	}
	return &info, nil
}

// UpsertWallet retrieves the latest wallet info from the ledger service, upserting the local database copy
func (service *Service) UpsertWallet(ctx context.Context, walletID uuid.UUID) (*wallet.Info, error) {
	wallet, err := service.LedgerClient.GetWallet(ctx, walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error looking up wallet")
	}
	if wallet != nil {
		err = service.Datastore.UpsertWallet(wallet)
		if err != nil {
			return nil, errorutils.Wrap(err, "error saving wallet")
		}
	}
	return wallet, nil
}

// SubmitAnonCardTransaction validates and submits a transaction on behalf of an anonymous card
func (service *Service) SubmitAnonCardTransaction(
	ctx context.Context,
	walletID uuid.UUID,
	transaction string,
	destination string,
) (*wallet.TransactionInfo, error) {
	info, err := service.GetOrCreateWallet(ctx, walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}
	return service.SubmitCommitableAnonCardTransaction(ctx, info, transaction, destination, true)
}

// SubmitCommitableAnonCardTransaction submits a transaction
func (service *Service) SubmitCommitableAnonCardTransaction(
	ctx context.Context,
	info *walletutils.Info,
	transaction string,
	destination string,
	confirm bool,
) (*wallet.TransactionInfo, error) {
	providerWallet, err := provider.GetWallet(ctx, *info)
	if err != nil {
		return nil, err
	}
	anonCard, ok := providerWallet.(*uphold.Wallet)
	if !ok {
		return nil, errors.New("Only uphold wallets are supported")
	}

	// FIXME needs to require the idempotency key
	_, err = anonCard.VerifyAnonCardTransaction(transaction, destination)
	if err != nil {
		return nil, err
	}

	// Submit and confirm since we are requiring the idempotency key
	return anonCard.SubmitTransaction(transaction, confirm)
}
