package promotion

import (
	"context"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Drain ad suggestions into verified wallet
func (service *Service) Drain(ctx context.Context, credentials []CredentialBinding, walletID uuid.UUID) error {
	wallet, err := service.datastore.GetWallet(walletID)
	if err != nil || wallet == nil {
		return errors.Wrap(err, "Error getting wallet")
	}

	// A verified wallet will have a payout address
	if wallet.PayoutAddress == nil {
		// Try to retrieve updated wallet from the ledger service
		wallet, err = service.wallet.UpsertWallet(ctx, walletID)
		if err != nil {
			return errors.Wrap(err, "Error upserting wallet")
		}

		if wallet.PayoutAddress == nil {
			return errors.New("Wallet is not verified")
		}
	}

	// Iterate through each credential and assemble list of funding sources
	_, _, fundingSources, err := service.GetCredentialRedemptions(ctx, credentials)
	if err != nil {
		return err
	}

	for _, v := range fundingSources {
		if v.Type != "ads" {
			return errors.New("Only ads suggestions can be drained")
		}

		var promotion Promotion
		promotion.ID = v.PromotionID
		claim, err := service.datastore.GetClaimByWalletAndPromotion(wallet, &promotion)
		if err != nil || claim == nil {
			return errors.Wrap(err, "Error finding claim for wallet")
		}

		if v.Amount.GreaterThan(claim.ApproximateValue) {
			return errors.New("Cannot claim more funds than were earned")
		}

		// Skip already drained promotions for idempotency
		if !claim.Drained {
			// Mark corresponding claim as drained
			err := service.datastore.DrainClaim(claim, v.Credentials, wallet, v.Amount)
			if err != nil {
				return errors.Wrap(err, "Error draining claim")
			}
		}
	}

	return nil
}

// DrainWorker attempts to work on a drain job by redeeming the credentials and transferring funds
type DrainWorker interface {
	RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, walletID uuid.UUID, total decimal.Decimal) (*wallet.TransactionInfo, error)
}

// RedeemAndTransferFunds after validating that all the credential bindings
func (service *Service) RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, walletID uuid.UUID, total decimal.Decimal) (*wallet.TransactionInfo, error) {
	wallet, err := service.datastore.GetWallet(walletID)
	if err != nil {
		return nil, err
	}

	if wallet == nil || wallet.PayoutAddress == nil {
		return nil, errors.New("missing wallet")
	}

	err = service.cbClient.RedeemCredentials(ctx, credentials, walletID.String())
	if err != nil {
		return nil, err
	}

	// FIXME
	//return grantWallet.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(total), wallet.PayoutAddress)
	return nil, nil
}
