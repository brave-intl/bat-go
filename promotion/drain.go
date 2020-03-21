package promotion

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/wallet"
	raven "github.com/getsentry/raven-go"
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
	_, _, fundingSources, promotions, err := service.GetCredentialRedemptions(ctx, credentials)
	if err != nil {
		return err
	}

	for k, v := range fundingSources {
		if v.Type != "ads" {
			return errors.New("Only ads suggestions can be drained")
		}

		fmt.Println(k)
		fmt.Println(v)
		fmt.Println(promotions)

		promotion := promotions[k]

		claim, err := service.datastore.GetClaimByWalletAndPromotion(wallet, promotion)
		if err != nil || claim == nil {
			return errors.Wrap(err, "Error finding claim for wallet")
		}

		suggestionsExpected, err := claim.SuggestionsNeeded(promotion)
		if err != nil {
			return fmt.Errorf("error calculating expected number of suggestions: %w", err)
		}

		amountExpected := decimal.New(int64(suggestionsExpected), 0).Mul(promotion.CredentialValue())
		if v.Amount.GreaterThan(amountExpected) {
			return errors.New("Cannot claim more funds than were earned")
		}

		// Skip already drained promotions for idempotency
		if !claim.Drained {
			// Mark corresponding claim as drained
			err := service.datastore.DrainClaim(claim, v.Credentials, wallet, v.Amount)
			if err != nil {
				return errors.Wrap(err, "Error draining claim")
			}

			go func() {
				_, err := service.RunNextDrainJob(ctx)
				if err != nil {
					raven.CaptureErrorAndWait(err, nil)
				}
			}()
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

	// FIXME should use idempotency key
	tx, err := service.hotWallet.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(total), *wallet.PayoutAddress)
	if err != nil {
		return nil, err
	}

	if service.drainChannel != nil {
		service.drainChannel <- tx
	}

	return tx, err
}
