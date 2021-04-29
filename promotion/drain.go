package promotion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	sentry "github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	errMissingTransferPromotion = errors.New("missing configuration: BraveTransferPromotionID")
	errGeminiMisconfigured      = errors.New("gemini is not configured")
	errReputationServiceFailure = errors.New("failed to call reputation service")
	errWalletNotReputable       = errors.New("wallet is not reputable")
)

// Drain ad suggestions into verified wallet
func (service *Service) Drain(ctx context.Context, credentials []CredentialBinding, walletID uuid.UUID) (*uuid.UUID, error) {
	var batchID = uuid.NewV4()

	wallet, err := service.wallet.Datastore.GetWallet(ctx, walletID)
	if err != nil || wallet == nil {
		return nil, fmt.Errorf("error getting wallet: %w", err)
	}

	// A verified wallet will have a payout address
	if wallet.UserDepositDestination == "" {
		return nil, errors.New("wallet is not verified")
	}

	// Iterate through each credential and assemble list of funding sources
	_, _, fundingSources, promotions, err := service.GetCredentialRedemptions(ctx, credentials)
	if err != nil {
		return nil, err
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
		// first let's make sure this wallet is an ios attested device...

		ctx = context.WithValue(ctx, appctx.WalletOnPlatformPriorToCTXKey, os.Getenv("WALLET_ON_PLATFORM_PRIOR_TO"))
		// is this from wallet reputable as an iOS device?
		isFromOnPlatform, err := service.reputationClient.IsWalletOnPlatform(ctx, walletID, "ios")
		if err != nil {
			return nil, fmt.Errorf("invalid device: %w", err)
		}

		if !isFromOnPlatform {
			// wallet is not reputable, decline
			return nil, fmt.Errorf("unable to drain to wallet: invalid device")
		}

		// these drained claims commit to mint
		var promotionIDs = []uuid.UUID{}
		for k := range fundingSources {
			promotionIDs = append(promotionIDs, promotions[k].ID)
		}

		walletID, err := uuid.FromString(wallet.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid wallet id: %w", err)
		}

		err = service.Datastore.EnqueueMintDrainJob(ctx, walletID, promotionIDs...)
		if err != nil {
			return nil, fmt.Errorf("error adding mint drain: %w", err)
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
			return nil, errors.New("only ads suggestions can be drained")
		}

		claim, err := service.Datastore.GetClaimByWalletAndPromotion(wallet, promotion)
		if err != nil || claim == nil {
			return nil, fmt.Errorf("error finding claim for wallet: %w", err)
		}

		suggestionsExpected, err := claim.SuggestionsNeeded(promotion)
		if err != nil {
			return nil, fmt.Errorf("error calculating expected number of suggestions: %w", err)
		}

		amountExpected := decimal.New(int64(suggestionsExpected), 0).Mul(promotion.CredentialValue())
		if v.Amount.GreaterThan(amountExpected) {
			return nil, errors.New("cannot claim more funds than were earned")
		}

		// Skip already drained promotions for idempotency
		if !claim.Drained {
			// Mark corresponding claim as drained
			err := service.Datastore.DrainClaim(&batchID, claim, v.Credentials, wallet, v.Amount)
			if err != nil {
				return nil, fmt.Errorf("error draining claim: %w", err)
			}

			// the original request context will be cancelled as soon as the dialer closes the connection.
			// this will setup a new context with the same values and a 90 second timeout
			asyncCtx, asyncCancel := context.WithTimeout(context.Background(), 90*time.Second)
			scopedCtx := appctx.Wrap(ctx, asyncCtx)

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

				_, err := service.RunNextDrainJob(scopedCtx)
				if err != nil {
					sentry.CaptureException(err)
				}
			}()
		}
	}
	if depositProvider == "brave" && wallet.UserDepositDestination != "" {
		asyncCtx, asyncCancel := context.WithTimeout(context.Background(), 90*time.Second)
		scopedCtx := appctx.Wrap(ctx, asyncCtx)

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

			_, err := service.RunNextMintDrainJob(scopedCtx)
			if err != nil {
				sentry.CaptureException(err)
			}
		}()
	}
	return &batchID, nil
}

// DrainPoll - Response structure for the DrainPoll
type DrainPoll struct {
	ID     *uuid.UUID `db:"id"`
	Status string     `db:"status"`
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

	if ok, _ := appctx.GetBoolFromContext(ctx, appctx.ReputationOnDrainCTXKey); ok {
		// perform reputation check for wallet, and error accordingly if there is a reputation failure
		reputable, err := service.reputationClient.IsWalletAdsReputable(ctx, walletID, "")
		if err != nil {
			logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to check reputation of wallet")
			return nil, errReputationServiceFailure
		}

		if !reputable {
			return nil, errWalletNotReputable
		}
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
	} else if *wallet.UserDepositAccountProvider == "bitflyer" {

		transferID := uuid.NewV4().String()

		totalF64, _ := total.Float64()

		// get quote, make sure we dont go over 100K JPY
		quote, err := service.bfClient.FetchQuote(ctx, "BAT_JPY", false)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch bitflyer quote")
		}

		JPYLimit := decimal.NewFromFloat(100000)
		var overLimitErr error

		totalJPYTransfer := total.Mul(quote.Rate)

		if totalJPYTransfer.GreaterThan(JPYLimit) {
			over := JPYLimit.Sub(totalJPYTransfer).String()
			totalF64, _ = JPYLimit.Div(quote.Rate).Floor().Float64()
			overLimitErr = fmt.Errorf("transfer is over 100K JPY by %s; BAT_JPY rate: %v; BAT: %v", over, quote.Rate, total)
		}

		tx := new(walletutils.TransactionInfo)

		tx.ID = transferID
		tx.Destination = wallet.UserDepositDestination
		tx.DestAmount = total

		// create a WithdrawToDepositIDBulkPayload
		payload := bitflyer.WithdrawToDepositIDBulkPayload{
			Withdrawals: []bitflyer.WithdrawToDepositIDPayload{
				{
					CurrencyCode: "BAT",
					Amount:       totalF64,
					DepositID:    wallet.UserDepositDestination,
					TransferID:   transferID,
					SourceFrom:   "userdrain",
				},
			},
		}
		// upload
		_, err = service.bfClient.UploadBulkPayout(ctx, payload)
		if err != nil {
			// if this was a bitflyer error and the error is due to a 401 response, refresh the token
			var bfe *clients.BitflyerError
			if errors.As(err, &bfe) {
				if bfe.Status == http.StatusUnauthorized {
					// try to refresh the token and go again
					logger.Warn().Msg("attempting to refresh the bf token")
					_, err = service.bfClient.RefreshToken(ctx, bitflyer.TokenPayloadFromCtx(ctx))
					if err != nil {
						return nil, fmt.Errorf("failed to get token from bf: %w", err)
					}
					// redo the request after token refresh
					_, err := service.bfClient.UploadBulkPayout(ctx, payload)
					if err != nil {
						return nil, fmt.Errorf("failed to transfer funds: %w", err)
					}
				}

				for _, v := range bfe.ErrorIDs {
					// non-retry errors, report to sentry
					if v == "NO_INV" {
						logger.Error().Err(bfe).Msg("no bitflyer inventory")
						sentry.CaptureException(bfe)
					}
				}
				// runner has ability to read ErrorIDs from bfe and code it
				return nil, bfe
			}
			return nil, fmt.Errorf("failed to transfer funds: %w", err)
		}
		// check if this

		if service.drainChannel != nil {
			service.drainChannel <- tx
		}

		if overLimitErr != nil {
			return tx, overLimitErr
		}

		return tx, err
	} else if *wallet.UserDepositAccountProvider == "gemini" {

		return redeemAndTransferGeminiFunds(ctx, service, wallet, total)

	} else if *wallet.UserDepositAccountProvider == "brave" {
		// update the mint job for this walletID

		promoTotal := map[string]decimal.Decimal{}
		// iterate through the credentials
		// get a total count per promotion
		for _, cred := range credentials {
			promotionID := strings.TrimSuffix(cred.Issuer, ":control")
			v, ok := promoTotal[promotionID]
			if ok {
				// each credential is 0.25
				promoTotal[promotionID] = v.Add(decimal.NewFromFloat(0.25))
			} else {
				promoTotal[promotionID] = decimal.NewFromFloat(0.25)
			}
		}
		for k, v := range promoTotal {
			promotionID, err := uuid.FromString(k)
			if err != nil {
				return nil, fmt.Errorf("failed to get promotion id as uuid: %w", err)
			}
			// update the mint_drain_promotion table with the corresponding total redeemed
			err = service.Datastore.SetMintDrainPromotionTotal(ctx, walletID, promotionID, v)
			if err != nil {
				return nil, fmt.Errorf("failed to set append total funds: %w", err)
			}
		}
		return new(walletutils.TransactionInfo), nil
	}

	logger.Error().Msg("RedeemAndTransferFunds: unknown deposit provider")
	return nil, fmt.Errorf(
		"failed to transfer funds: user_deposit_account_provider unknown: %s",
		*wallet.UserDepositAccountProvider)
}

func redeemAndTransferGeminiFunds(
	ctx context.Context,
	service *Service,
	wallet *walletutils.Info,
	total decimal.Decimal,
) (*walletutils.TransactionInfo, error) {

	// in the event that gemini configs or service do not exist
	// error on redeem and transfer
	if service.geminiConf == nil || service.geminiClient == nil {
		return errGeminiMisconfigured
	}

	ns, err := uuid.FromString(wallet.UserDepositDestination)
	if err != nil {
		return nil, fmt.Errorf("invalid user deposit destination: %w", err)
	}
	txType := "drain"
	channel := "wallet"
	transferID := uuid.NewV5(ns, txType+channel).String()

	tx := new(walletutils.TransactionInfo)

	tx.ID = transferID
	tx.Destination = wallet.UserDepositDestination
	tx.DestAmount = total

	account := "primary" // the account we want to drain from
	settlementTx := settlement.Transaction{
		SettlementID: transferID,
		Type:         txType,
		Destination:  wallet.UserDepositDestination,
		Channel:      channel,
	}
	payouts := []gemini.PayoutPayload{
		{
			TxRef:       gemini.GenerateTxRef(&settlementTx),
			Amount:      total,
			Currency:    "BAT",
			Destination: wallet.UserDepositDestination,
			Account:     &account,
		},
	}

	payload := gemini.NewBulkPayoutPayload(
		&account,
		service.geminiConf.ClientID,
		&payouts,
	)
	// upload
	signer := cryptography.NewHMACHasher([]byte(service.geminiConf.Secret))
	serializedPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize payload: %w", err)
	}
	b64Payload := base64.StdEncoding.EncodeToString([]byte(serializedPayload))
	_, err = service.geminiClient.UploadBulkPayout(
		ctx,
		service.geminiConf.APIKey,
		signer,
		b64Payload,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer funds: %w", err)
	}

	// check if we have a drainChannel defined on our service
	if service.drainChannel != nil {
		service.drainChannel <- tx
	}
	return tx, err
}

// MintGrant create a new grant for the wallet specified with the total specified
func (service *Service) MintGrant(ctx context.Context, walletID uuid.UUID, total decimal.Decimal, promotions ...uuid.UUID) error {
	// setup a logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		_, logger = logging.SetupLogger(ctx)
	}

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
