package wallet

import (
	"context"
	"errors"
	"net/http"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/wallet"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	walletClaimNamespace = uuid.Must(uuid.FromString("c39b298b-b625-42e9-a463-69c7726e5ddc"))
)

// Service contains datastore connections
type Service struct {
	Datastore   Datastore
	RoDatastore ReadOnlyDatastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context, datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	service := &Service{
		Datastore:   datastore,
		RoDatastore: roDatastore,
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

// SubmitAnonCardTransaction validates and submits a transaction on behalf of an anonymous card
func (service *Service) SubmitAnonCardTransaction(
	ctx context.Context,
	walletID uuid.UUID,
	transaction string,
	destination string,
) (*wallet.TransactionInfo, error) {
	info, err := service.Datastore.GetWallet(walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}
	return service.SubmitCommitableAnonCardTransaction(ctx, info, transaction, destination, true)
}

// GetWallet - get a wallet by id
func (service *Service) GetWallet(ID uuid.UUID) (*walletutils.Info, error) {
	return service.Datastore.GetWallet(ID)
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

// LinkWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkWallet(
	ctx context.Context,
	wallet uphold.Wallet,
	transaction string,
	anonymousAddress *uuid.UUID,
) error {
	// do not confirm this transaction yet
	info := wallet.GetWalletInfo()

	var (
		userID          string
		depositProvider string
		probi           decimal.Decimal
	)

	tx, err := wallet.VerifyTransaction(transaction)
	if err != nil {
		return handlers.WrapError(errors.New("failed to verify transaction"), "failed to verify transaction", http.StatusForbidden)
	}

	// verify that the user is kyc from uphold. (for all wallet provider cases)
	if uID, ok, err := wallet.IsUserKYC(ctx, tx.Destination); err != nil {
		// there was an error
		return handlers.WrapError(err,
			"wallet could not be kyc checked",
			http.StatusInternalServerError,
		)
	} else if !ok {
		// fail
		return &handlers.AppError{
			Cause: errors.New("user kyc did not pass"),
			Code:  http.StatusForbidden,
		}
	} else {
		userID = uID
	}

	// check kyc user id validity
	if userID == "" {
		err := errors.New("user id not provided")
		return handlers.WrapError(err, "unable to link wallet", http.StatusBadRequest)
	}

	probi = tx.Probi
	depositProvider = "uphold"

	providerLinkingID := uuid.NewV5(walletClaimNamespace, userID)
	if info.ProviderLinkingID != nil {
		// check if the member matches the associated member
		if !uuid.Equal(*info.ProviderLinkingID, providerLinkingID) {
			return handlers.WrapError(errors.New("wallets do not match"), "unable to match wallets", http.StatusForbidden)
		}
	} else {
		// tx.Destination will be stored as "ProviderID"
		err := service.Datastore.LinkWallet(info.ID, tx.Destination, providerLinkingID, anonymousAddress, depositProvider)
		if err != nil {
			status := http.StatusInternalServerError
			if err == ErrTooManyCardsLinked {
				status = http.StatusConflict
			}
			return handlers.WrapError(err, "unable to link wallets", status)
		}
	}

	// if this wallet is linking a deposit account do not submit a transaction
	if decimal.NewFromFloat(0).LessThan(probi) {
		_, err := service.SubmitCommitableAnonCardTransaction(ctx, &info, transaction, "", true)
		if err != nil {
			return handlers.WrapError(err, "unable to transfer tokens", http.StatusBadRequest)
		}
	}
	return nil
}
