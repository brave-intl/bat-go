package promotion

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/wallet"
	sentry "github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Drain ad suggestions into verified wallet
func (service *Service) Drain(ctx context.Context, credentials []CredentialBinding, walletID uuid.UUID) error {
	wallet, err := service.wallet.Datastore.GetWallet(walletID)
	if err != nil || wallet == nil {
		return fmt.Errorf("error getting wallet: %w", err)
	}

	// A verified wallet will have a payout address
	if wallet.UserDepositDestination == "" {
		return errors.New("Wallet is not verified")
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

		promotion := promotions[k]

		claim, err := service.Datastore.GetClaimByWalletAndPromotion(wallet, promotion)
		if err != nil || claim == nil {
			return fmt.Errorf("error finding claim for wallet: %w", err)
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
			err := service.Datastore.DrainClaim(claim, v.Credentials, wallet, v.Amount)
			if err != nil {
				return fmt.Errorf("error draining claim: %w", err)
			}

			go func() {
				defer middleware.ConcurrentGoRoutines.With(
					prometheus.Labels{
						"method": "NextDrainJob",
					}).Dec()

				middleware.ConcurrentGoRoutines.With(
					prometheus.Labels{
						"method": "NextDrainJob",
					}).Inc()
				_, err := service.RunNextDrainJob(ctx)
				if err != nil {
					sentry.CaptureException(err)
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
	wallet, err := service.wallet.Datastore.GetWallet(walletID)
	if err != nil {
		return nil, err
	}

	// no wallet on record
	if wallet == nil {
		return nil, errorutils.ErrMissingWallet
	}

	// wallet not linked to deposit destination, if absent fail redeem and transfer
	if wallet.UserDepositDestination == "" {
		return nil, errorutils.ErrNoDepositProviderDestination
	}
	if wallet.UserDepositAccountProvider == nil {
		return nil, errorutils.ErrNoDepositProviderDestination
	}

	// failed to redeem credentials
	if err = service.cbClient.RedeemCredentials(ctx, credentials, walletID.String()); err != nil {
		return nil, fmt.Errorf("failed to redeem credentials: %w", err)
	}

	if *wallet.UserDepositAccountProvider == "brave" {
		// get and parse the correct transfer promotion id to create claims on
		braveTransferPromotionID, ok := ctx.Value(appctx.BraveTransferPromotionIDCTXKey).(string)
		if !ok {
			return nil, errors.New("missing configuration: BraveTransferPromotionID")
		}
		pID, err := uuid.FromString(braveTransferPromotionID)
		if err != nil {
			return nil, fmt.Errorf("invalid configuration, unable to parse BraveTransferPromotionID: %w", err)
		}
		// create a new claim for the wallet deposit account for total
		_, err = service.Datastore.CreateClaim(pID, wallet.UserDepositDestination, total, decimal.Zero)
		return nil, err
	} else if *wallet.UserDepositAccountProvider == "uphold" {
		// FIXME should use idempotency key
		tx, err := service.hotWallet.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(total), wallet.UserDepositDestination)
		if err != nil {
			return nil, fmt.Errorf("failed to transfer funds: %w", err)
		}
		if service.drainChannel != nil {
			service.drainChannel <- tx
		}
		return tx, err
	}

	return nil, fmt.Errorf(
		"failed to transfer funds: user_deposit_account_provider unknown: %s",
		*wallet.UserDepositAccountProvider)
}
