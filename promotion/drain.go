package promotion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	w "github.com/brave-intl/bat-go/utils/wallet"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	contextutil "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	sentry "github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var errMissingTransferPromotion = errors.New("missing configuration: BraveTransferPromotionID")

// Drain ad suggestions into verified wallet
func (service *Service) Drain(ctx context.Context, credentials []CredentialBinding, walletID uuid.UUID) error {
	wallet, err := service.wallet.Datastore.GetWallet(ctx, walletID)
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
	var (
		depositProvider string
	)
	if wallet.UserDepositAccountProvider != nil {
		depositProvider = *wallet.UserDepositAccountProvider
	}

	// if this is a brave wallet with a user deposit destination, we need to create a
	// mint drain job in waiting status, waiting for all promotions to be added to it
	if depositProvider == "brave" && wallet.UserDepositDestination != "" {
		// these drained claims commit to mint
		var promotionIDs = []uuid.UUID{}
		for k := range fundingSources {
			promotionIDs = append(promotionIDs, promotions[k].ID)
		}

		walletID, err := uuid.FromString(wallet.ID)
		if err != nil {
			return fmt.Errorf("invalid wallet id: %w", err)
		}

		err = service.Datastore.EnqueueMintDrainJob(ctx, walletID, promotionIDs...)
		if err != nil {
			return fmt.Errorf("error adding mint drain: %w", err)
		}
	}

	for k, v := range fundingSources {
		var (
			promotion = promotions[k]
		)

		// if the type is not ads
		// except in the case the promotion is for ios and deposit provider is a brave wallet
		if v.Type != "ads" &&
			depositProvider != "brave" && strings.ToLower(promotion.Platform) != "ios" {
			return errors.New("Only ads suggestions can be drained")
		}

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

			// the original request context will be cancelled as soon as the dialer closes the connection.
			// this will setup a new context with the same values and a minute timeout
			asyncCtx, asyncCancel := context.WithTimeout(context.Background(), time.Minute)
			ctx = contextutil.Wrap(ctx, asyncCtx)

			go func() {
				defer asyncCancel()
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

	// if this is a brave deposit provider, with a deposit destination, we need to
	// commit the mint drain job by setting it's status to pending
	if depositProvider == "brave" && wallet.UserDepositDestination != "" {
		// this will setup a new context with the same values and a minute timeout
		asyncCtx, asyncCancel := context.WithTimeout(context.Background(), time.Minute)
		ctx = contextutil.Wrap(ctx, asyncCtx)

		go func() {
			defer asyncCancel()
			defer middleware.ConcurrentGoRoutines.With(
				prometheus.Labels{
					"method": "NextMintDrainJob",
				}).Dec()

			middleware.ConcurrentGoRoutines.With(
				prometheus.Labels{
					"method": "NextMintDrainJob",
				}).Inc()

			_, err := service.RunNextMintDrainJob(ctx)
			if err != nil {
				sentry.CaptureException(err)
			}
		}()
	}

	return nil
}

// DrainWorker attempts to work on a drain job by redeeming the credentials and transferring funds
type DrainWorker interface {
	RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, walletID uuid.UUID, total decimal.Decimal) (*walletutils.TransactionInfo, error)
}

// MintWorker mint worker describes what a mint worker is able to do, mint grants
type MintWorker interface {
	MintGrant(ctx context.Context, walletID uuid.UUID, total decimal.Decimal, promoIDs ...uuid.UUID) error
}

// RedeemAndTransferFunds after validating that all the credential bindings
func (service *Service) RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, walletID uuid.UUID, total decimal.Decimal) (*walletutils.TransactionInfo, error) {

	// setup a logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	wallet, err := service.wallet.Datastore.GetWallet(ctx, walletID)
	if err != nil {
		logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to get wallet")
		return nil, err
	}

	// no wallet on record
	if wallet == nil {
		logger.Error().Err(errorutils.ErrMissingWallet).
			Msg("RedeemAndTransferFunds: missing wallet")
		return nil, errorutils.ErrMissingWallet
	}

	// wallet not linked to deposit destination, if absent fail redeem and transfer
	if wallet.UserDepositDestination == "" {
		logger.Error().Err(errorutils.ErrNoDepositProviderDestination).
			Msg("RedeemAndTransferFunds: no deposit provider destination")
		return nil, errorutils.ErrNoDepositProviderDestination
	}
	if wallet.UserDepositAccountProvider == nil {
		logger.Error().Msg("RedeemAndTransferFunds: no deposit provider")
		return nil, errorutils.ErrNoDepositProviderDestination
	}

	// failed to redeem credentials
	if err = service.cbClient.RedeemCredentials(ctx, credentials, walletID.String()); err != nil {
		logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to redeem credentials")
		return nil, fmt.Errorf("failed to redeem credentials: %w", err)
	}

	if *wallet.UserDepositAccountProvider == "uphold" {
		// FIXME should use idempotency key
		tx, err := service.hotWallet.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(total), wallet.UserDepositDestination)
		if err != nil {
			return nil, fmt.Errorf("failed to transfer funds: %w", err)
		}
		if service.drainChannel != nil {
			service.drainChannel <- tx
		}
		return tx, err
	} else if *wallet.UserDepositAccountProvider == "brave" {
		// this will be handled by the mint drain job
		return new(w.TransactionInfo), nil
	}

	logger.Error().Msg("RedeemAndTransferFunds: unknown deposit provider")
	return nil, fmt.Errorf(
		"failed to transfer funds: user_deposit_account_provider unknown: %s",
		*wallet.UserDepositAccountProvider)
}

// MintGrant create a new grant for the wallet specified with the total specified
func (service *Service) MintGrant(ctx context.Context, walletID uuid.UUID, total decimal.Decimal, promotions ...uuid.UUID) error {
	// setup a logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		_, logger = logging.SetupLogger(ctx)
	}

	logger.Warn().Msg("here, in mintgrant!")

	// for all of the promotion ids (limit of 4 wallets can be linked)
	// attempt to create a claim.  If we run into a unique key constraint, this means that
	// we have already created a claim for this wallet id/ promotion
	var attempts int
	for _, pID := range promotions {
		logger.Debug().Msg("MintGrant: creating the claim to destination")
		// create a new claim for the wallet deposit account for total
		// this is a legacy claimed claim
		_, err = service.Datastore.CreateClaim(pID, walletID.String(), total, decimal.Zero, true)
		if err != nil {
			var pgErr *pq.Error
			if errors.As(err, &pgErr) {
				// unique constraint error (wallet id and promotion id combo exists)
				// use one of the other 4 promotions instead
				if pgErr.Code == "23505" {
					attempts++
					continue
				}
			}
			logger.Error().Err(err).Msg("MintGrant: failed to create a new claim to destination")
			return err
		}
		break
	}
	if attempts >= len(promotions) {
		return errors.New("limit of draining 4 wallets to brave wallet exceeded")
	}
	return nil
}
