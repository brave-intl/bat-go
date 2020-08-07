package promotion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
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
	if wallet.AnonymousAddress == nil && wallet.ProviderID == "" {
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

	if wallet == nil || (wallet.AnonymousAddress == nil && wallet.ProviderID == "") {
		return nil, errors.New("missing wallet")
	}

	err = service.cbClient.RedeemCredentials(ctx, credentials, walletID.String())
	if err != nil {
		return nil, err
	}

	enableLinkingDraining, err := strconv.ParseBool(os.Getenv("ENABLE_LINKING_DRAINING"))
	if err != nil {
		return nil, fmt.Errorf("invalid enable_linking_draining flag: %w", err)
	}

	if enableLinkingDraining {
		// first use the provider id, if not exists use anon address
		var destination = wallet.ProviderID

		if destination == "" {
			if wallet.AnonymousAddress != nil {
				destination = wallet.AnonymousAddress.String()
			}
		}

		if destination != "" {
			// FIXME should use idempotency key
			tx, err := service.hotWallet.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(total), destination)
			if err != nil {
				return nil, err
			}
			if service.drainChannel != nil {
				service.drainChannel <- tx
			}

			return tx, err
		}
		return nil, errors.New("no anonymous address for wallet for transfer")
	}
	return nil, errors.New("linking and draining is disabled")
}
