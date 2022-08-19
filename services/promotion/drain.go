package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/ptr"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/clients/reputation"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

const (
	txnStatusGeminiPending string = "gemini-pending"
)

var (
	errMissingTransferPromotion = errors.New("missing configuration: BraveTransferPromotionID")
	errGeminiMisconfigured      = errors.New("gemini is not configured")
	errReputationServiceFailure = errors.New("failed to call reputation service")
	errWalletNotReputable       = errors.New("wallet is not reputable")
	errWalletDrainLimitExceeded = errors.New("wallet drain limit exceeded")
	withdrawalLimitHit          = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "withdrawalLimitHit",
			Help:        "A counter for when a drain hits the withdrawal limit",
			ConstLabels: prometheus.Labels{"service": "wallet"},
		})
)

// Drain ad suggestions into verified wallet
func (service *Service) Drain(ctx context.Context, credentials []CredentialBinding, walletID uuid.UUID) (*uuid.UUID, error) {

	logger := logging.Logger(ctx, "promotion.Drain")

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

	drainClaims := make([]DrainClaim, 0)
	for k, v := range fundingSources {
		var promotion = promotions[k]

		// if the type is not ads
		// except in the case the promotion is for ios and deposit provider is a brave wallet
		if v.Type != "ads" && depositProvider != "brave" && strings.ToLower(promotion.Platform) != "ios" {
			sublogger.Error().Msg("invalid promotion platform, must be ads")
			continue
		}

		claim, err := service.Datastore.GetClaimByWalletAndPromotion(wallet, promotion)
		if err != nil || claim == nil {
			sublogger.Error().Err(err).Str("promotion_id", promotion.ID.String()).Msg("claim does not exist for wallet")
			// the case where there this wallet never got this promotion
			drainClaims = append(drainClaims, DrainClaim{
				BatchID:     &batchID,
				Claim:       claim,
				Credentials: v.Credentials,
				Wallet:      wallet,
				Total:       v.Amount,
				CodedErr:    errMismatchedWallet,
			})
			continue
		}

		suggestionsExpected, err := claim.SuggestionsNeeded(promotion)
		if err != nil {
			sublogger.Error().Err(err).Str("promotion_id", promotion.ID.String()).Msg("invalid number of suggestions")
			// the case where there is an invalid number of suggestions
			drainClaims = append(drainClaims, DrainClaim{
				BatchID:     &batchID,
				Claim:       claim,
				Credentials: v.Credentials,
				Wallet:      wallet,
				Total:       v.Amount,
				CodedErr:    errInvalidSuggestionCount,
			})
			continue
		}

		amountExpected := decimal.New(int64(suggestionsExpected), 0).Mul(promotion.CredentialValue())
		if v.Amount.GreaterThan(amountExpected) {
			sublogger.Error().Str("promotion_id", promotion.ID.String()).Msg("attempting to claim more funds than earned")
			// the case where there the amount is higher than expected
			drainClaims = append(drainClaims, DrainClaim{
				BatchID:     &batchID,
				Claim:       claim,
				Credentials: v.Credentials,
				Wallet:      wallet,
				Total:       v.Amount,
				CodedErr:    errInvalidSuggestionAmount,
			})
			continue
		}

		// skip already drained promotions for idempotency
		if !claim.Drained {
			drainClaims = append(drainClaims, DrainClaim{
				BatchID:     &batchID,
				Claim:       claim,
				Credentials: v.Credentials,
				Wallet:      wallet,
				Total:       v.Amount,
				CodedErr:    nil,
			})
		}
	}

	if len(drainClaims) > 0 {
		err = service.Datastore.DrainClaims(drainClaims)
		if err != nil {
			return nil, fmt.Errorf("faied to insert drain claims for walletID %s: %w", walletID, err)
		}
	}

	for i := 0; i < len(drainClaims); i++ {
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
	RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, drainJob DrainJob) (*walletutils.TransactionInfo, error)
}

// DrainRetryWorker - reads walletID
type DrainRetryWorker interface {
	FetchAdminAttestationWalletID(ctx context.Context) (*uuid.UUID, error)
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

// GeminiTxnStatusWorker this worker retrieves the status for a given gemini transaction
type GeminiTxnStatusWorker interface {
	GetGeminiTxnStatus(ctx context.Context, txRef string) (*walletutils.TransactionInfo, error)
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
	drainCodeErrorInvalidDepositID = errorutils.Codified{
		ErrCode: "invalid_deposit_id",
		Retry:   false,
	}
)

// bitflyerOverTransferLimit - a error bundle "codified" implemented "data" field for error bundle
// providing the specific drain code for the drain job error codification
type bitflyerOverTransferLimit struct{}

func (botl *bitflyerOverTransferLimit) DrainCode() (string, bool) {
	return "bf_transfer_limit", true
}

// SubmitBatchTransfer after validating that all the credential bindings
func (service *Service) SubmitBatchTransfer(ctx context.Context, batchID *uuid.UUID) error {
	// setup a logger
	logger := logging.Logger(ctx, "promotion.SubmitBatchTransfer")

	// TODO: when nitro enablement we will perform tx submissions here
	// but for now we will perform the bf client bulk upload
	/*
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
	*/
	// for now we will only be batching bitflyer txs

	// get quote, make sure we dont go over 100K JPY
	quote, err := service.bfClient.FetchQuote(ctx, "BAT_JPY", false)
	if err != nil {
		// if this was a bitflyer error and the error is due to a 401 response, refresh the token
		var bfe *clients.BitflyerError
		if errors.As(err, &bfe) {
			if bfe.HTTPStatusCode == http.StatusUnauthorized {
				// try to refresh the token and go again
				logger.Warn().Msg("attempting to refresh the bf token")
				_, err = service.bfClient.RefreshToken(ctx, bitflyer.TokenPayloadFromCtx(ctx))
				if err != nil {
					return fmt.Errorf("failed to get token from bf: %w", err)
				}
				// redo the request after token refresh
				quote, err = service.bfClient.FetchQuote(ctx, "BAT_JPY", false)
				if err != nil {
					return fmt.Errorf("failed to fetch bitflyer quote: %w", err)
				}
			}
		} else {
			// unknown error
			return fmt.Errorf("failed to fetch bitflyer quote: %w", err)
		}
	}

	JPYLimit := decimal.NewFromFloat(100000)
	var overLimitErr error

	// get all transactions associated with batch id
	transfers, err := service.Datastore.GetDrainsByBatchID(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to get transactions for batch: %w", err)
	}
	var (
		withdraws        = []bitflyer.WithdrawToDepositIDPayload{}
		totalJPYTransfer = decimal.Zero
	)

	var (
		totalF64  float64
		depositID string
	)

	for _, v := range transfers {

		if v.DepositID == nil {
			return errorutils.New(fmt.Errorf("failed depositID cannot be nil for batchID %s", batchID),
				"submit batch transfer", drainCodeErrorInvalidDepositID)
		}

		// set deposit id for the transfer
		depositID = *v.DepositID

		t, _ := v.Total.Float64()
		totalF64 += t

		totalJPYTransfer = totalJPYTransfer.Add(v.Total.Mul(quote.Rate))
		if totalJPYTransfer.GreaterThan(JPYLimit) {
			over := JPYLimit.Sub(totalJPYTransfer).String()
			totalF64, _ = JPYLimit.Div(quote.Rate).Floor().Float64()
			overLimitErr = errorutils.New(
				fmt.Errorf(
					"over custodian transfer limit - JPY by %s; BAT_JPY rate: %v; BAT: %v",
					over, quote.Rate, totalJPYTransfer),
				"over custodian transfer limit",
				new(bitflyerOverTransferLimit))
			break
		}
	}

	// collapse into one transaction, not multiples in a bulk upload

	withdraws = append(withdraws, bitflyer.WithdrawToDepositIDPayload{
		CurrencyCode: "BAT",
		Amount:       totalF64,
		DepositID:    depositID,
		TransferID:   batchID.String(),
		SourceFrom:   "userdrain",
	})

	// create a WithdrawToDepositIDBulkPayload
	payload := bitflyer.WithdrawToDepositIDBulkPayload{
		Withdrawals: withdraws,
	}

	withdrawToDepositIDBulkResponse, err := service.bfClient.UploadBulkPayout(ctx, payload)
	if err != nil {
		var bitflyerError *clients.BitflyerError

		switch {
		case errors.As(err, &bitflyerError):

			// if this was a bitflyer 401 response refresh the token and retry upload otherwise return bitflyer error
			if bitflyerError.HTTPStatusCode == http.StatusUnauthorized {
				logger.Warn().Msg("attempting to refresh the bf token")
				_, err = service.bfClient.RefreshToken(ctx, bitflyer.TokenPayloadFromCtx(ctx))
				if err != nil {
					return fmt.Errorf("failed to get token from bf: %w", err)
				}
				withdrawToDepositIDBulkResponse, err = service.bfClient.UploadBulkPayout(ctx, payload)
				if err != nil {
					return fmt.Errorf("failed to transfer funds: %w", err)
				}
			} else {
				return bitflyerError
			}

		default:
			return fmt.Errorf("failed to transfer funds: %w", err)
		}
	}

	if withdrawToDepositIDBulkResponse == nil || len(withdrawToDepositIDBulkResponse.Withdrawals) == 0 {
		return fmt.Errorf("submit batch transfer error: response cannot be nil for batchID %s", batchID)
	}

	// check the txn for errors
	for _, withdrawal := range withdrawToDepositIDBulkResponse.Withdrawals {
		if withdrawal.CategorizeStatus() == "failed" {

			err = fmt.Errorf("submit batch transfer error: bitflyer %s error for batchID %s",
				withdrawal.Status, withdrawal.TransferID)

			retry := true
			if withdrawal.Status == "NO_INV" {
				retry = false
			}

			codified := errorutils.Codified{
				ErrCode: fmt.Sprintf("bitflyer_%s", strings.ToLower(withdrawal.Status)),
				Retry:   retry,
			}

			return errorutils.New(err, "submit batch transfer", codified)
		}
	}

	if overLimitErr != nil {
		return overLimitErr
	}

	return nil
}

// RedeemAndTransferFunds after validating that all the credential bindings
func (service *Service) RedeemAndTransferFunds(ctx context.Context, credentials []cbr.CredentialRedemption, drainJob DrainJob) (*walletutils.TransactionInfo, error) {

	// setup a logger
	logger := logging.Logger(ctx, "promotion.RedeemAndTransferFunds")

	wallet, err := service.wallet.Datastore.GetWallet(ctx, drainJob.WalletID)
	if err != nil {
		logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to get wallet")
		return nil, err
	}

	defer func() {
		if err != nil {
			custodian := "unknown"
			if wallet != nil && ptr.String(wallet.UserDepositAccountProvider) != "" {
				custodian = *wallet.UserDepositAccountProvider
			}
			countClaimDrainStatus.
				With(prometheus.Labels{"custodian": custodian,
					"status": "failed"}).Inc()
		}
	}()

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
		if err = service.cbClient.RedeemCredentials(ctx, credentials, drainJob.WalletID.String()); err != nil {
			logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to redeem credentials")
			return nil, fmt.Errorf("failed to redeem credentials: %w", err)
		}
	}

	if ok, _ := appctx.GetBoolFromContext(ctx, appctx.ReputationOnDrainCTXKey); ok {
		// are we running in withdrawal limit mode?
		if ok, _ := appctx.GetBoolFromContext(ctx, appctx.ReputationWithdrawalOnDrainCTXKey); ok {
			// tally up all prior claims on this promotion for all linked provider accounts associated
			// as the "withdrawalAmount"
			promotionID, withdrawalAmount, err := service.Datastore.GetWithdrawalsAssociated(&drainJob.WalletID, drainJob.ClaimID)
			if err != nil {
				logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to lookup associated withdrawals")
				return nil, fmt.Errorf("failed to lookup associated withdrawals: %w", err)
			}

			if promotionID == nil {
				logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to lookup associated withdrawals")
				return nil, fmt.Errorf("failed to lookup associated withdrawals: no matching promotion")
			}

			// perform reputation check for wallet, and error accordingly if there is a reputation failure
			reputable, cohorts, err := service.reputationClient.IsDrainReputable(ctx, drainJob.WalletID, *promotionID, withdrawalAmount)
			if err != nil {
				logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to check reputation of wallet")
				return nil, errReputationServiceFailure
			}

			if !reputable {
				// use the cohort to determine the limit exceeded.
				for _, cohort := range cohorts {
					switch cohort {
					case reputation.CohortWithdrawalLimits:
						// limited withdrawal
						withdrawalLimitHit.Inc()
						return nil, errWalletDrainLimitExceeded
					case reputation.CohortNil:
						// service failure
						return nil, errReputationServiceFailure
					}
				}
				// not reputable
				countDrainFlaggedUnusual.Inc()
				return nil, errWalletNotReputable
			}
		} else {
			// legacy behavior
			// perform reputation check for wallet, and error accordingly if there is a reputation failure
			reputable, err := service.reputationClient.IsWalletAdsReputable(ctx, drainJob.WalletID, "")
			if err != nil {
				logger.Error().Err(err).Msg("RedeemAndTransferFunds: failed to check reputation of wallet")
				return nil, errReputationServiceFailure
			}

			if !reputable {
				return nil, errWalletNotReputable
			}
		}
	}

	if *wallet.UserDepositAccountProvider == "uphold" {
		// FIXME should use idempotency key
		tx, err := service.hotWallet.Transfer(ctx, altcurrency.BAT, altcurrency.BAT.ToProbi(drainJob.Total), wallet.UserDepositDestination)
		if err != nil {
			return nil, fmt.Errorf("failed to transfer funds: %w", err)
		}
		if service.drainChannel != nil {
			service.drainChannel <- tx
		}
		return tx, err
	} else if *wallet.UserDepositAccountProvider == "bitflyer" {
		return redeemAndTransferBitflyerFunds(ctx, service, wallet, drainJob.Total)
	} else if *wallet.UserDepositAccountProvider == "gemini" {
		return redeemAndTransferGeminiFunds(ctx, service, drainJob.Total, drainJob)
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
			err = service.Datastore.SetMintDrainPromotionTotal(ctx, drainJob.WalletID, promotionID, v)
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

func redeemAndTransferBitflyerFunds(
	ctx context.Context,
	service *Service,
	wallet *walletutils.Info,
	total decimal.Decimal,
) (*walletutils.TransactionInfo, error) {

	transferID := uuid.NewV4().String()

	tx := new(walletutils.TransactionInfo)

	tx.ID = transferID
	tx.Destination = wallet.UserDepositDestination
	tx.DestAmount = total
	tx.Status = "bitflyer-consolidate"

	// Actual Transfer is now done in SubmitBatchTransfer worker
	// job will be marked as completed

	if service.drainChannel != nil {
		service.drainChannel <- tx
	}

	return tx, nil
}

func redeemAndTransferGeminiFunds(ctx context.Context, service *Service, total decimal.Decimal, drainJob DrainJob) (*walletutils.TransactionInfo, error) {

	// in the event that gemini configs or service do not exist
	// error on redeem and transfer
	if service.geminiConf == nil {
		return nil, fmt.Errorf("missing gemini client and configuration: %w", errGeminiMisconfigured)
	}

	txType := "drain"
	channel := "wallet"
	transferID := uuid.NewV4().String()

	depositDestination := ptr.String(drainJob.DepositDestination)
	if depositDestination == "" {
		return nil, fmt.Errorf("error deposit destination is nil for drain job %s", drainJob.ID.String())
	}

	tx := new(walletutils.TransactionInfo)
	tx.ID = transferID
	tx.Destination = depositDestination
	tx.DestAmount = total
	tx.Status = txnStatusGeminiPending

	settlementTx := custodian.Transaction{
		SettlementID: transferID,
		Type:         txType,
		Destination:  depositDestination,
		Channel:      channel,
	}

	account := "primary" // the account we want to drain from
	payouts := []gemini.PayoutPayload{
		{
			TxRef:       gemini.GenerateTxRef(&settlementTx),
			Amount:      total,
			Currency:    "BAT",
			Destination: depositDestination,
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
	// gemini client will base64 encode the payload prior to sending
	resp, err := service.geminiClient.UploadBulkPayout(
		ctx,
		service.geminiConf.APIKey,
		signer,
		string(serializedPayload),
	)

	if err != nil {
		var eb *errorutils.ErrorBundle
		if errors.As(err, &eb) {
			// retrieve the error bundle data if there is any and log
			errorData := eb.DataToString()
			logging.FromContext(ctx).Error().
				Err(eb.Cause()).
				Interface("wallet_id", drainJob.WalletID).
				Str("error_bundle", errorData).
				Msg("failed to transfer funds gemini")
		}
		return nil, fmt.Errorf("failed to transfer funds: %w", err)
	}

	if resp == nil || len(*resp) < 1 {
		// failed to get a response from the server
		return nil, fmt.Errorf("failed to transfer funds: gemini 'result' is not OK")
	}

	// for all the submitted, check they are all okay
	for _, payout := range *resp {

		logging.FromContext(ctx).Info().
			Interface("wallet_id", drainJob.WalletID).
			Str("tx_ref", payout.TxRef).
			Str("payout_result", payout.Result).
			Str("payout_status", ptr.StringOr(payout.Status, "unknown_status")).
			Str("payout_reason", ptr.StringOr(payout.Reason, "no_reason")).
			Msg("checking gemini submitted transactions")

		if strings.ToLower(payout.Result) != "ok" {
			return nil, fmt.Errorf("failed to transfer funds: gemini 'result' is not OK: %s",
				ptr.StringOr(payout.Reason, "unknown reason"))
		}
		if strings.ToLower(ptr.String(payout.Status)) == "failed" {
			return nil, fmt.Errorf("failed to transfer funds: gemini payout status failed: %s",
				ptr.StringOr(payout.Reason, "unknown reason"))
		}
	}

	// used for testing only
	if service.drainChannel != nil {
		service.drainChannel <- tx
	}

	return tx, err
}

// MintGrant create a new grant for the wallet specified with the total specified
func (service *Service) MintGrant(ctx context.Context, walletID uuid.UUID, total decimal.Decimal, promotions ...uuid.UUID) error {
	// setup a logger
	logger := logging.Logger(ctx, "promotion.MintGrant")

	// for all of the promotion ids (limit of 4 wallets can be linked)
	// attempt to create a claim.  If we run into a unique key constraint, this means that
	// we have already created a claim for this wallet id/ promotion
	var attempts int
	for _, pID := range promotions {
		logger.Debug().Msg("MintGrant: creating the claim to destination")
		// create a new claim for the wallet deposit account for total
		// this is a legacy claimed claim
		_, err := service.Datastore.CreateClaim(pID, walletID.String(), total, decimal.Zero, true)
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

// FetchAdminAttestationWalletID - retrieves walletID from topic
func (service *Service) FetchAdminAttestationWalletID(ctx context.Context) (*uuid.UUID, error) {
	message, err := service.kafkaAdminAttestationReader.ReadMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("read message: error reading kafka message %w", err)
	}

	codec, ok := service.codecs[adminAttestationTopic]
	if !ok {
		return nil, fmt.Errorf("read message: could not find codec %s", adminAttestationTopic)
	}

	native, _, err := codec.NativeFromBinary(message.Value)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode naitve from binary %w", err)
	}

	textual, err := codec.TextualFromNative(nil, native)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode textual from native %w", err)
	}

	var adminAttestationEvent AdminAttestationEvent
	err = json.Unmarshal(textual, &adminAttestationEvent)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode json from textual %w", err)
	}

	walletID := uuid.FromStringOrNil(adminAttestationEvent.WalletID)
	if walletID == uuid.Nil {
		return nil, fmt.Errorf("read message: error could not decode walletID %s", adminAttestationEvent.WalletID)
	}

	return &walletID, nil
}

// GetGeminiTxnStatus retrieves the status for a given gemini transaction
func (service *Service) GetGeminiTxnStatus(ctx context.Context, txRef string) (*walletutils.TransactionInfo, error) {
	apiKey, ok := ctx.Value(appctx.GeminiAPIKeyCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("no gemini api key in ctx: %w", appctx.ErrNotInContext)
	}

	clientID, ok := ctx.Value(appctx.GeminiClientIDCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("no gemini browser client id in ctx: %w", appctx.ErrNotInContext)
	}

	payoutResult, err := service.geminiClient.CheckTxStatus(ctx, apiKey, clientID, txRef)
	if err != nil {
		var errorBundle *errorutils.ErrorBundle
		if errors.As(err, &errorBundle) {
			errorData := errorBundle.DataToString()
			logging.FromContext(ctx).Error().
				Err(errorBundle.Cause()).
				Str("txRef", txRef).
				Str("error_bundle", errorData).
				Msg("gemini client check status error")

			if httpState, ok := errorBundle.Data().(clients.HTTPState); ok {
				if httpState.Status == http.StatusNotFound {
					return &walletutils.TransactionInfo{Status: "failed", Note: "GEMINI_NOT_FOUND"}, nil
				}
			}
		}
		return nil, fmt.Errorf("failed to check gemini txn status for %s: %w", txRef, err)
	}

	if payoutResult == nil {
		return nil, fmt.Errorf("failed to get gemini txn status for %s: response nil", txRef)
	}

	if strings.ToLower(payoutResult.Result) == "error" {
		return nil, fmt.Errorf("failed to get gemini txn status for %s: %s", txRef,
			ptr.StringOr(payoutResult.Reason, "unknown gemini response error"))
	}

	switch strings.ToLower(ptr.String(payoutResult.Status)) {
	case "completed":
		return &walletutils.TransactionInfo{Status: "complete"}, nil
	case "pending", "processing":
		return &walletutils.TransactionInfo{Status: "pending"}, nil
	case "failed":
		return &walletutils.TransactionInfo{Status: "failed", Note: ptr.String(payoutResult.Reason)}, nil
	}

	return nil, fmt.Errorf("failed to get txn status for %s: unknown status %s",
		txRef, ptr.String(payoutResult.Status))
}
