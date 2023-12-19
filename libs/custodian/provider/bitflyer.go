package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	loggingutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/gofrs/uuid"
	"github.com/shopspring/decimal"
)

var (
	// BitflyerTransferIDNamespace uuidv5 namespace for bitflyer transfer id creation
	BitflyerTransferIDNamespace = uuid.Must(uuid.FromString("5b208c1d-e1c4-4799-bcc2-0b08b9c660f5"))
	// ErrInvalidSource - invalid source for bitflyer
	ErrInvalidSource = errors.New("invalid source for bitflyer")
)

// bitflyerCustodian - implementation of the bitflyer custodian
type bitflyerCustodian struct {
	client bitflyer.Client
}

// newBitflyerCustodian - create a new bitflyer custodian with configuration
func newBitflyerCustodian(ctx context.Context, conf Config) (*bitflyerCustodian, error) {
	logger := loggingutils.Logger(ctx, "custodian.newBitflyerCustodian").With().Str("conf", conf.String()).Logger()

	// import config to context if not already set, and create bitflyer client
	bfClient, err := bitflyer.NewWithContext(appctx.MapToContext(ctx, conf.Config))
	if err != nil {
		msg := "failed to create client"
		return nil, loggingutils.LogAndError(&logger, msg, fmt.Errorf("%s: %w", msg, err))
	}

	return &bitflyerCustodian{
		client: bfClient,
	}, nil
}

func isBitflyerErrorUnauthorized(ctx context.Context, err error) bool {
	var bfe *clients.BitflyerError
	if errors.As(err, &bfe) {
		return bfe.HTTPStatusCode == http.StatusUnauthorized
	}
	var bundleErr *errorutils.ErrorBundle
	if errors.As(err, &bundleErr) {
		if state, ok := bundleErr.Data().(clients.HTTPState); ok {
			return state.Status == http.StatusUnauthorized
		}
	}
	return false
}

// SubmitTransactions - implement Custodian interface
func (bc *bitflyerCustodian) SubmitTransactions(ctx context.Context, txs ...Transaction) error {
	// setup logger
	logger := loggingutils.Logger(ctx, "bitflyerCustodian.SubmitTransactions")

	// we need to check the limit total for all txs that they do not exceed a limit JPY
	var (
		JPYLimit         = decimal.NewFromFloat(100000)
		totalJPYTransfer = decimal.Zero
		// collapsed transactions per destination
		destTxs = map[string][]Transaction{}
		// list of quote rates per currency
		currencies = map[string]decimal.Decimal{}
	)

	for i := range txs {
		// add the currency to the list of all currencies we need rates for
		currency, err := txs[i].GetCurrency(ctx)
		if err != nil {
			msg := "failed to get tx currency"
			return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
		}

		// set this as a currency we need to get a rate for
		currencies[currency.String()] = decimal.Zero

		// collapse down based on deposit id
		destination, err := txs[i].GetDestination(ctx)
		if err != nil {
			msg := "failed to get tx destination"
			return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
		}

		destTxs[destination.String()] = append(destTxs[destination.String()], txs[i])
	}

	// get all the quotes for each of the currencies
	for k := range currencies {
		quote, err := bc.client.FetchQuote(ctx, fmt.Sprintf("%s_JPY", k), false)
		if err != nil {
			if isBitflyerErrorUnauthorized(ctx, err) {
				// if this was a bitflyer error and the error is due to a 401 response, refresh the token
				logger.Debug().Msg("attempting to refresh the bf token")
				_, err = bc.client.RefreshToken(ctx, bitflyer.TokenPayloadFromCtx(ctx))
				if err != nil {
					msg := "failed to get token from bitflyer.RefreshToken"
					return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
				}
				// redo the request after token refresh
				quote, err = bc.client.FetchQuote(ctx, fmt.Sprintf("%s_JPY", k), false)
				if err != nil {
					msg := "failed to fetch bitflyer quote"
					return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
				}
			} else {
				// non-recoverable error fetching quote
				msg := "failed to fetch bitflyer quote"
				return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
			}
		}

		// set the rate for the currency
		currencies[k] = quote.Rate
	}

	// each destination must have all txes collapsed according to bf
	var destConsolidatedTxs = []bitflyer.WithdrawToDepositIDPayload{}

	// for each destination we need to collapse all of the transactions into one
	for destination, txs := range destTxs {
		// to make a combined idempotency key
		var (
			transferIDs  = []string{}
			c            string
			totalF64     float64
			limitReached bool
		)

		for _, tx := range txs {
			idempotencyKey, err := tx.GetIdempotencyKey(ctx)
			if err != nil {
				// non-recoverable error getting tx amount
				msg := "failed to get idempotency key from transaction"
				return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
			}
			transferIDs = append(transferIDs, idempotencyKey.String())

			if !limitReached {
				// get the amount of the tx
				amount, err := tx.GetAmount(ctx)
				if err != nil {
					// non-recoverable error getting tx amount
					msg := "failed to get amount from transaction"
					return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
				}

				t, _ := amount.Float64()
				// we cap at the total if we exceed...
				totalF64 += t

				// lookup the currency string of this TX so we can figure out if we are over
				currency, err := tx.GetCurrency(ctx)
				if err != nil {
					// non-recoverable error getting tx currency
					msg := "failed to get currency from transaction"
					return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
				}

				c = currency.String()

				totalJPYTransfer = totalJPYTransfer.Add(amount.Mul(currencies[currency.String()]))

				if totalJPYTransfer.GreaterThan(JPYLimit) {
					over := JPYLimit.Sub(totalJPYTransfer).String()
					totalF64, _ = JPYLimit.Div(currencies[currency.String()]).Floor().Float64()

					overLimitErr := fmt.Errorf(
						"over custodian transfer limit - JPY by %s; %s_JPY rate: %v; BAT: %v",
						over, currency.String(), currencies[currency.String()], totalJPYTransfer)

					logger.Warn().Err(overLimitErr).Msg("destination exceeds jpy limit")
					limitReached = true
				}
			}
		}

		sourceFrom, ok := ctx.Value(appctx.BitflyerSourceFromCTXKey).(string)
		if !ok {
			// bad configuration, need source from specified
			msg := "failed to get source from context"
			return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, ErrInvalidSource))
		}

		// sort the transferIDs so we can recalculate the transfer_id for this batch of txs
		sort.Strings(transferIDs)

		// set up a client payload
		destConsolidatedTxs = append(destConsolidatedTxs, bitflyer.WithdrawToDepositIDPayload{
			CurrencyCode: c,
			Amount:       totalF64,
			DepositID:    destination,
			// transferids uuidv5 with namespace
			// deterministic from all tx transfers
			TransferID: uuid.NewV5(BitflyerTransferIDNamespace, strings.Join(transferIDs, ",")).String(),
			SourceFrom: sourceFrom,
		})
	}

	// finally send all these txs to bf
	// create a WithdrawToDepositIDBulkPayload
	payload := bitflyer.WithdrawToDepositIDBulkPayload{
		Withdrawals: destConsolidatedTxs,
	}
	// upload
	_, err := bc.client.UploadBulkPayout(ctx, payload)
	if err != nil {
		// non-recoverable error getting tx currency
		msg := "failed to perform bulk payout with bitflyer"
		return loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
	}

	return nil
}

type bitflyerTransactionStatus struct {
	Status  string
	Message string
}

func (bts *bitflyerTransactionStatus) String() string {
	return bts.Status
}

// GetTransactionsStatus - implement Custodian interface.  within this implementation
// a business requirement of the transaction submission is to collapse all txs per destination
// into one transfer.  To do this we take all of the txs in the passed in batch, organize by
// destination, then collapse a sorted list of idempotency keys into an array which is used
// to create a unified batch transfer_id
func (bc *bitflyerCustodian) GetTransactionsStatus(ctx context.Context, txs ...Transaction) (map[string]TransactionStatus, error) {
	logger := loggingutils.Logger(ctx, "bitflyerCustodian.GetTransactionsStatus")
	var (
		transferIDs = []string{}
		destTxs     = map[string][]Transaction{}
	)

	// collapse by deposit destination
	for i := range txs {
		// collapse down based on deposit id
		destination, err := txs[i].GetDestination(ctx)
		if err != nil {
			msg := "failed to get tx destination"
			return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
		}

		destTxs[destination.String()] = append(destTxs[destination.String()], txs[i])
	}

	for _, txs := range destTxs {
		// to make a combined idempotency key
		var (
			idempotencyKeys = []string{}
		)
		for _, tx := range txs {
			idempotencyKey, err := tx.GetIdempotencyKey(ctx)
			if err != nil {
				// non-recoverable error getting tx amount
				msg := "failed to get idempotency key from transaction"
				return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
			}
			// add to transferIDs
			idempotencyKeys = append(idempotencyKeys, idempotencyKey.String())
		}
		// sort the idempotency keys so we can recalculate the transfer_id for this batch of txs
		sort.Strings(idempotencyKeys)

		transferIDs = append(transferIDs, uuid.NewV5(BitflyerTransferIDNamespace, strings.Join(idempotencyKeys, ",")).String())
	}

	resp, err := bc.client.CheckPayoutStatus(ctx, bitflyer.TransferIDsToBulkStatus(transferIDs))
	if err != nil {
		if isBitflyerErrorUnauthorized(ctx, err) {
			// if this was a bitflyer error and the error is due to a 401 response, refresh the token
			logger.Debug().Msg("attempting to refresh the bf token")
			_, err = bc.client.RefreshToken(ctx, bitflyer.TokenPayloadFromCtx(ctx))
			if err != nil {
				msg := "failed to get token from bitflyer.RefreshToken"
				return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
			}
			// redo the request after token refresh
			resp, err = bc.client.CheckPayoutStatus(ctx, bitflyer.TransferIDsToBulkStatus(transferIDs))
			if err != nil {
				msg := "failed to fetch bitflyer transaction status"
				return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
			}
		} else {
			// non-recoverable error fetching quote
			msg := "failed to fetch bitflyer transaction status"
			return nil, loggingutils.LogAndError(logger, msg, fmt.Errorf("%s: %w", msg, err))
		}
	}
	logger.Debug().Str("resp", fmt.Sprintf("%+v", resp)).Msg("response from check payout status")

	var txStatuses = map[string]TransactionStatus{}
	// reconstruct our response
	if resp != nil {
		if resp.Withdrawals != nil {
			for _, v := range resp.Withdrawals {
				txStatuses[v.TransferID] = &bitflyerTransactionStatus{
					Status: v.Status, Message: v.Message,
				}
			}
		}
	}

	logger.Debug().Str("txStatuses", fmt.Sprintf("%+v", txStatuses)).Msg("result of statuses")

	return txStatuses, nil
}
