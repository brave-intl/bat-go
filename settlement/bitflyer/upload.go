package bitflyersettlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/shopspring/decimal"
)

// GroupSettlements groups settlements under a single provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	settlements *[]settlement.Transaction,
) map[string][]settlement.Transaction {
	grouped := make(map[string][]settlement.Transaction)
	for _, payout := range *settlements {
		if payout.WalletProvider == "bitflyer" {
			id := bitflyer.GenerateTransferID(&payout)
			grouped[id] = append(grouped[id], payout)
		}
	}

	return grouped
}

// CategorizeResponse categorizes a response from bitflyer as pending, complete, failed, or unknown
func CategorizeResponse(
	originalTransactions map[string][]settlement.Transaction,
	payout *bitflyer.WithdrawToDepositIDResponse,
) ([]settlement.Transaction, string) {
	txs := originalTransactions[payout.TransferID]
	key := "unknown"
	switch payout.Status {
	case "SUCCESS", "EXECUTED":
		key = "complete"
	case "NOT_FOUND", "NO_INV", "INVALID_MEMO", "NOT_FOUNTD", "INVALID_AMOUNT", "NOT_ALLOWED_TO_SEND", "NOT_ALLOWED_TO_RECV", "LOCKED_BY_QUICK_DEPOSIT", "SESSION_SEND_LIMIT", "SESSION_TIME_OUT", "EXPIRED", "NOPOSITION", "OTHER_ERROR":
		key = "failed"
	case "CREATED", "PENDING":
		key = "pending"
	}
	aggregatedAmount := decimal.Zero
	for i, original := range txs {
		original.Status = key
		note := payout.Status
		if payout.Message != "" {
			note = fmt.Sprintf("%s: %s", payout.Status, payout.Message)
		}
		original.Note = note
		aggregatedAmount = aggregatedAmount.Add(original.Probi)
		tmp := altcurrency.BAT
		original.AltCurrency = &tmp
		original.Currency = tmp.String()
		txs[i] = original
	}
	if key == "complete" && !payout.Amount.Equal(decimal.Zero) {
		aggregatedAmount = altcurrency.BAT.FromProbi(aggregatedAmount)
		if !aggregatedAmount.Equal(decimal.Zero) && !aggregatedAmount.Equal(payout.Amount) {
			key = "invalid-input"
			for i := range txs {
				txs[i].Status = key
			}
		}
	}
	return txs, key
}

// CategorizeResponses categorizes the series of responses
func CategorizeResponses(
	originalTransactions map[string][]settlement.Transaction,
	response *[]bitflyer.WithdrawToDepositIDResponse,
) map[string][]settlement.Transaction {
	transactions := make(map[string][]settlement.Transaction)

	for _, payout := range *response {
		original, key := CategorizeResponse(
			originalTransactions,
			&payout,
		)
		nonZero := []settlement.Transaction{}
		for _, tx := range original {
			if tx.Probi.GreaterThan(decimal.Zero) {
				nonZero = append(nonZero, tx)
			}
		}
		transactions[key] = append(transactions[key], nonZero...)
	}
	return transactions
}

// SubmitBulkPayoutTransactions submits bulk payout transactions
func SubmitBulkPayoutTransactions(
	ctx context.Context,
	transactionsMap map[string][]settlement.Transaction,
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements bitflyer.WithdrawToDepositIDBulkPayload,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]settlement.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	logging.SubmitProgress(ctx, blockProgress, total)

	logger.Debug().
		Int("total", total).
		Int("progress", blockProgress).
		Msg("sending request")

	response, err := bitflyerClient.UploadBulkPayout(
		ctx,
		bulkPayoutRequestRequirements,
	)
	<-time.After(time.Second)
	if err != nil {
		logger.Error().Err(err).Msg("error performing upload")
		return submittedTransactions, err
	}
	// collect all successful transactions to send to eyeshade
	submitted := CategorizeResponses(transactionsMap, &response.Withdrawals)
	for key, txs := range submitted {
		submittedTransactions[key] = append(submittedTransactions[key], txs...)
	}
	return submittedTransactions, nil
}

// CheckPayoutTransactionsStatus checks the status of given transactions
func CheckPayoutTransactionsStatus(
	ctx context.Context,
	transactionsMap map[string][]settlement.Transaction,
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements bitflyer.WithdrawToDepositIDBulkPayload,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]settlement.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	result, err := bitflyerClient.CheckPayoutStatus(
		ctx,
		bulkPayoutRequestRequirements,
	)
	if err != nil {
		return nil, err
	}
	response := CategorizeResponses(
		transactionsMap,
		&result.Withdrawals,
	)
	for key, original := range response {
		submittedTransactions[key] = append(submittedTransactions[key], original...)
		prog := blockProgress
		logging.SubmitProgress(ctx, prog, total)
		logger.Debug().
			Int("total", total).
			Int("progress", prog).
			Str("key", key).
			Str("tx_ref", original[0].DocumentID).
			Msg("parameters used")
	}
	return submittedTransactions, err
}

func setupSettlementTransactions(
	transactions map[string][]settlement.Transaction,
	limit decimal.Decimal,
) (
	// transfer_id => partial transactions
	map[string][]settlement.Transaction,
	*[][]settlement.Transaction,
	error,
) {
	settlementRequests := [][]settlement.Transaction{{}}
	settlements := make(map[string][]settlement.Transaction)
	for key, groupedWithdrawals := range transactions {
		set, index := getSettlementGroup(&settlementRequests)
		aggregatedTx := settlement.Transaction{}
		for gwdIndex, wd := range groupedWithdrawals {
			// last := 1+gwdIndex == len(groupedWithdrawals)
			if gwdIndex == 0 {
				aggregatedTx.AltCurrency = wd.AltCurrency
				aggregatedTx.Currency = wd.Currency
				aggregatedTx.Destination = wd.Destination
				aggregatedTx.Publisher = wd.Publisher
				aggregatedTx.WalletProvider = wd.WalletProvider
				aggregatedTx.WalletProviderID = wd.WalletProviderID
				aggregatedTx.ProviderID = wd.WalletProviderID
				aggregatedTx.Channel = wd.Channel
				aggregatedTx.SettlementID = wd.SettlementID
				aggregatedTx.Type = wd.Type
			}
			partialProbi := wd.Probi
			// will hit our limits
			if aggregatedTx.Amount.Add(partialProbi).GreaterThan(limit) {
				// reduce amount and fee to be within. can be zero
				partialProbi = limit.Sub(aggregatedTx.Probi)
			}
			partialFee := decimal.Zero
			if wd.BATPlatformFee.GreaterThan(decimal.Zero) {
				partialFee = partialProbi.Div(decimal.NewFromFloat(19))
			}
			partialAmount := altcurrency.BAT.FromProbi(partialProbi.Add(partialFee)) // always in BAT to BAT so we're good
			// add to aggregate provider transaction
			aggregatedTx.Amount = aggregatedTx.Amount.Add(partialAmount)
			aggregatedTx.BATPlatformFee = aggregatedTx.BATPlatformFee.Add(partialFee) // not needed but useful for sanity checking
			aggregatedTx.Probi = aggregatedTx.Probi.Add(partialProbi)                 // not needed but useful for sanity checking
			// need separate so we can settle different types on eyeshade
			// update single settlement.
			wd.Amount = partialAmount
			wd.BATPlatformFee = partialFee
			wd.Probi = partialProbi
			// attach to upper levels
			wd.ProviderID = key
			settlements[key] = append(settlements[key], wd)
		}
		*set = append(*set, aggregatedTx)
		settlementRequests[index] = *set
	}
	return settlements, &settlementRequests, nil
}

func createBitflyerRequests(
	sourceFrom string,
	dryRun *bitflyer.DryRunOption,
	token string,
	settlementRequests *[][]settlement.Transaction,
) (*[]bitflyer.WithdrawToDepositIDBulkPayload, error) {
	bitflyerRequests := []bitflyer.WithdrawToDepositIDBulkPayload{}
	for _, withdrawalSet := range *settlementRequests {
		bitflyerPayloads, err := bitflyer.NewWithdrawsFromTxs(
			sourceFrom,
			&withdrawalSet,
		)
		if err != nil {
			return nil, err
		}
		bitflyerRequests = append(bitflyerRequests, *bitflyer.NewWithdrawToDepositIDBulkPayload(
			dryRun,
			token,
			bitflyerPayloads,
		))
	}
	return &bitflyerRequests, nil
}

func getSettlementGroup(
	settlementRequests *[][]settlement.Transaction,
) (*[]settlement.Transaction, int) {
	requestSeries := *settlementRequests
	lastIndex := len(requestSeries) - 1
	set := requestSeries[lastIndex]
	if len(requestSeries[lastIndex]) >= 1000 {
		set := []settlement.Transaction{}
		*settlementRequests = append(*settlementRequests, set)
		return &set, len(*settlementRequests) - 1
	}
	return &set, len(*settlementRequests) - 1
}

// IterateRequest iterates requests
func IterateRequest(
	ctx context.Context,
	action string,
	bitflyerClient bitflyer.Client,
	bulkPayoutFiles []string,
	sourceFrom string,
	dryRun *bitflyer.DryRunOption,
) (*map[string][]settlement.Transaction, error) {

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	// for submission to eyeshade
	submittedTransactions := make(map[string][]settlement.Transaction)

	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY")
	if err != nil {
		return &submittedTransactions, err
	}
	for _, bulkPayoutFile := range bulkPayoutFiles {
		bytes, err := ioutil.ReadFile(bulkPayoutFile)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read bulk payout file")
			return &submittedTransactions, err
		}

		var txs []settlement.Transaction
		err = json.Unmarshal(bytes, &txs)
		if err != nil {
			logger.Error().Err(err).Msg("failed unmarshal bulk payout file")
			return &submittedTransactions, err
		}
		// bat limit
		limit := altcurrency.BAT.ToProbi(decimal.NewFromFloat32(200000). // start with jpy
											Div(quote.Rate). // convert to bat
											Truncate(8))     // truncated to satoshis
		transactionsMap, transactionGroups, err := setupSettlementTransactions(
			GroupSettlements(&txs),
			limit,
		)
		if err != nil {
			return nil, err
		}

		requests, err := createBitflyerRequests(
			sourceFrom,
			dryRun,
			quote.PriceToken,
			transactionGroups,
		)
		if err != nil {
			return nil, err
		}

		for i, request := range *requests {
			if action == "upload" {
				submittedTransactions, err = SubmitBulkPayoutTransactions(
					ctx,
					transactionsMap,
					submittedTransactions,
					request,
					bitflyerClient,
					len(bulkPayoutFiles)-1,
					i,
				)
				if err != nil {
					logger.Error().Err(err).Msg("failed to submit bulk payout transactions")
					return nil, err
				}
			} else if action == "checkstatus" {
				submittedTransactions, err = CheckPayoutTransactionsStatus(
					ctx,
					transactionsMap,
					submittedTransactions,
					request,
					bitflyerClient,
					len(bulkPayoutFiles)-1,
					i,
				)
				if err != nil {
					logger.Error().Err(err).Msg("falied to check payout transactions status")
					return nil, err
				}
			}
		}
	}
	return &submittedTransactions, nil
}
