package promotion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/payments/pb"
	paymentspb "github.com/brave-intl/bat-go/payments/pb"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
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
	// TODO: move to payments
	//errGeminiMisconfigured      = errors.New("gemini is not configured")
	errReputationServiceFailure = errors.New("failed to call reputation service")
	errWalletNotReputable       = errors.New("wallet is not reputable")
)

// Drain ad suggestions into verified wallet
func (service *Service) Drain(ctx context.Context, credentials []CredentialBinding, walletID uuid.UUID) (*uuid.UUID, error) {

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	var batchID = uuid.NewV4()

	sublogger := logger.With().
		Str("wallet_id", walletID.String()).
		Str("batch_id", batchID.String()).
		Logger()

	wallet, err := service.wallet.Datastore.GetWallet(ctx, walletID)
	if err != nil || wallet == nil {
		sublogger.Error().Err(err).Msg("failed to get wallet by id")
		return nil, fmt.Errorf("error getting wallet: %w", err)
	}

	// A verified wallet will have a payout address
	if wallet.UserDepositDestination == "" {
		sublogger.Error().Err(err).Msg("wallet is not linked/verified")
		return nil, errors.New("wallet is not verified")
	}

	// Iterate through each credential and assemble list of funding sources
	_, _, fundingSources, promotions, err := service.GetCredentialRedemptions(ctx, credentials)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get credential redemptions")
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
			sublogger.Error().Err(err).Str("provider", "brave").Msg("wallet is not on ios platform")
			return nil, fmt.Errorf("invalid device: %w", err)
		}

		if !isFromOnPlatform {
			// wallet is not reputable, decline
			sublogger.Error().Str("provider", "brave").Msg("wallet is not on ios platform")
			return nil, fmt.Errorf("unable to drain to wallet: invalid device")
		}

		// these drained claims commit to mint
		var promotionIDs = []uuid.UUID{}
		for k := range fundingSources {
			promotionIDs = append(promotionIDs, promotions[k].ID)
		}

		walletID, err := uuid.FromString(wallet.ID)
		if err != nil {
			sublogger.Error().Str("provider", "brave").Msg("wallet id is invalid")
			return nil, fmt.Errorf("invalid wallet id: %w", err)
		}

		err = service.Datastore.EnqueueMintDrainJob(ctx, walletID, promotionIDs...)
		if err != nil {
			sublogger.Error().Str("provider", "brave").Msg("failed to add ios transfer job")
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
			sublogger.Error().Msg("invalid promotion platform, must be ads")
			return nil, errors.New("only ads suggestions can be drained")
		}

		claim, err := service.Datastore.GetClaimByWalletAndPromotion(wallet, promotion)
		if err != nil || claim == nil {
			sublogger.Error().Err(err).Str("promotion_id", promotion.ID.String()).Msg("claim does not exist for wallet")
			// the case where there this wallet never got this promotion
			return nil, service.Datastore.DrainClaim(&batchID, claim, v.Credentials, wallet, v.Amount, errMismatchedWallet)
		}

		suggestionsExpected, err := claim.SuggestionsNeeded(promotion)
		if err != nil {
			sublogger.Error().Err(err).Str("promotion_id", promotion.ID.String()).Msg("invalid number of suggestions")
			// the case where there is an invalid number of suggestions
			return nil, service.Datastore.DrainClaim(&batchID, claim, v.Credentials, wallet, v.Amount, errInvalidSuggestionCount)
		}

		amountExpected := decimal.New(int64(suggestionsExpected), 0).Mul(promotion.CredentialValue())
		if v.Amount.GreaterThan(amountExpected) {
			sublogger.Error().Str("promotion_id", promotion.ID.String()).Msg("attempting to claim more funds than earned")
			// the case where there the amount is higher than expected
			return nil, service.Datastore.DrainClaim(&batchID, claim, v.Credentials, wallet, v.Amount, errInvalidSuggestionAmount)
		}

		// Skip already drained promotions for idempotency
		if !claim.Drained {
			// Mark corresponding claim as drained
			err := service.Datastore.DrainClaim(&batchID, claim, v.Credentials, wallet, v.Amount, nil)
			if err != nil {
				sublogger.Error().Msg("failed to drain the claim")
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

// BatchTransferWorker - Worker that has the ability to "submit" a batch of transactions with payments service.
// The DrainWorker tasks employ the payments GRPC client "prepare" method, and provide the "batch id" in the
// metadata of the grpc request.  Payments GRPC server will append all TXs in a batch to a single transfer job.
// The SubmitBatchTransfer will notice claim_drain batches that are complete, and perform a submit to the Payments API
type BatchTransferWorker interface {
	SubmitBatchTransfer(ctx context.Context, batchID *uuid.UUID) error
}

// drainClaimErred - a codified err type for draind
type drainClaimErred struct {
	error
	Code string
}

// DrainCode - implement claim drain erred code
func (dce *drainClaimErred) DrainCode() (string, bool) {
	return dce.Code, false
}

var (
	errMismatchedWallet = &drainClaimErred{
		errors.New("claim does not exist for wallet"),
		"mismatched_wallet",
	}
	errInvalidSuggestionCount = &drainClaimErred{
		errors.New("invalid number of suggestions"),
		"invalid_suggestion_count",
	}
	errInvalidSuggestionAmount = &drainClaimErred{
		errors.New("attempting to claim more funds than earned"),
		"invalid_suggestion_amount",
	}
)

/* TODO: move to payments

// bitflyerOverTransferLimit - a error bundle "codified" implemented "data" field for error bundle
// providing the specific drain code for the drain job error codification
type bitflyerOverTransferLimit struct{}

func (botl *bitflyerOverTransferLimit) DrainCode() (string, bool) {
	return "bf_transfer_limit", true
}
*/

// SubmitBatchTransfer after validating that all the credential bindings
func (service *Service) SubmitBatchTransfer(ctx context.Context, batchID *uuid.UUID) error {
	// setup a logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	// use paymentsClient to "prepare" transfer with batch id
	_, err = service.paymentsClient.Submit(ctx, &paymentspb.SubmitRequest{
		BatchMeta: &paymentspb.BatchMeta{
			BatchId: batchID.String(),
		},
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to call submit to payments")
		return fmt.Errorf("failed to call submit for payments transfer: %w", err)
	}
	return service.Datastore.MarkBatchTransferSubmitted(ctx, batchID)
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

	// check to see if we skip the cbr redemption case
	if skipRedeem, _ := appctx.GetBoolFromContext(ctx, appctx.SkipRedeemCredentialsCTXKey); !skipRedeem {
		// failed to redeem credentials
		if err = service.cbClient.RedeemCredentials(ctx, credentials, walletID.String()); err != nil {
			logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to redeem credentials")
			return nil, fmt.Errorf("failed to redeem credentials: %w", err)
		}
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

	// use paymentsClient to "prepare" transfer with batch id
	resp, err := service.paymentsClient.Prepare(ctx, &paymentspb.PrepareRequest{
		BatchMeta: &paymentspb.BatchMeta{
			// TODO: get the batch id in here
		},
		Custodian: paymentspb.Custodian(paymentspb.Custodian_value[strings.ToUpper(*wallet.UserDepositAccountProvider)]),
		BatchTxs: []*pb.Transaction{
			{
				Destination: wallet.UserDepositDestination,
				Amount:      total.String(),
				Currency:    "BAT",
			},
		},
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to make connection to payments")
		return nil, fmt.Errorf("failed to call prepare for payments transfer: %w", err)
	}

	tx := new(walletutils.TransactionInfo)

	tx.ID = resp.DocumentIds[0]
	tx.Destination = wallet.UserDepositDestination
	tx.DestAmount = total

	if service.drainChannel != nil {
		service.drainChannel <- tx
	}
	return tx, nil
}

/* TODO - this gets moved into payments service proper

func redeemAndTransferGeminiFunds(
	ctx context.Context,
	service *Service,
	wallet *walletutils.Info,
	total decimal.Decimal,
) (*walletutils.TransactionInfo, error) {
	logger := logging.Logger(ctx, "redeemAndTransferGeminiFunds")

	// in the event that gemini configs or service do not exist
	// error on redeem and transfer
	if service.geminiConf == nil || service.geminiClient == nil {
		logger.Error().Msg("gemini is misconfigured, missing gemini client and configuration")
		return nil, errGeminiMisconfigured
	}

	txType := "drain"
	channel := "wallet"
	transferID := uuid.NewV4().String()

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
		logger.Error().Err(err).Msg("failed to serialize payload")
		return nil, fmt.Errorf("failed to serialize payload: %w", err)
	}
	// gemini client will base64 encode the payload prior to sending
	resp, err := service.geminiClient.UploadBulkPayout(
		ctx,
		service.geminiConf.APIKey,
		signer,
		string(serializedPayload),
	)

	if err != nil {
		logger.Error().Err(err).Msg("failed request to gemini")
		var eb *errorutils.ErrorBundle
		if errors.As(err, &eb) {
			// okay, there was an errorbundle, unwrap and log the error
			// convert err.Data() to json and report out
			b, err := json.Marshal(eb.Data())
			if err != nil {
				logger.Error().Err(err).Msg("failed serialize error bundle data")
			} else {
				logger.Error().Err(err).
					Str("data", string(b)).
					Msg("gemini client error details")
			}
		}
		return nil, fmt.Errorf("failed to transfer funds: %w", err)
	}

	if resp == nil || len(*resp) < 1 {
		// failed to get a response from the server
		return nil, fmt.Errorf("failed to transfer funds: gemini 'result' is not OK")
	}
	// for all the submitted, check they are all okay
	for _, v := range *resp {
		if strings.ToLower(v.Result) != "ok" {
			if v.Reason != nil {
				return nil, fmt.Errorf("failed to transfer funds: gemini 'result' is not OK: %s", *v.Reason)
			}
			return nil, fmt.Errorf("failed to transfer funds: gemini 'result' is not OK")
		}
	}

	// check if we have a drainChannel defined on our service
	if service.drainChannel != nil {
		service.drainChannel <- tx
	}
	return tx, err
}
*/

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
