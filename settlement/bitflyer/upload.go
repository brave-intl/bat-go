package bitflyersettlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/shopspring/decimal"
)

// CategorizeResponse categorizes a response from bitflyer as pending, complete, failed, or unknown
func CategorizeResponse(
	originalTransactions map[string][]settlement.Transaction,
	payout *bitflyer.WithdrawToDepositIDResponse,
) ([]settlement.Transaction, string) {
	txs := originalTransactions[payout.TransferID]
	k := "unknown"
	for i, original := range txs {
		key := "failed"
		if payout.Status == "Error" {
			original.Note = payout.Message
		} else {
			status := payout.Status
			key = "unknown"
			if payout.Status == "Pending" {
				key = "pending"
			} else if status == "Completed" {
				key = "complete"
			}
		}
		original.Status = key
		k = original.Status
		tmp := altcurrency.BAT
		original.AltCurrency = &tmp
		original.Currency = tmp.String()
		txs[i] = original
	}
	return txs, k
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
			if tx.Amount.GreaterThan(decimal.Zero) {
				nonZero = append(nonZero, tx)
			}
		}
		transactions[key] = append(transactions[key], original...)
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
	token string,
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
		Msg("parameters used")

	payload, err := json.Marshal(bulkPayoutRequestRequirements)
	if err != nil {
		return submittedTransactions, err
	}

	logger.Debug().
		Str("api key", token).
		Msg("sending request")

	response, err := bitflyerClient.UploadBulkPayout(
		ctx,
		token,
		payload,
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
	token string,
	total int,
	blockProgress int,
) (map[string][]settlement.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	payload, err := json.Marshal(bulkPayoutRequestRequirements)
	if err != nil {
		return nil, err
	}
	result, err := bitflyerClient.CheckPayoutStatus(
		ctx,
		token,
		payload,
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

// BitflyerWriteTransactions writes settlement transactions to a json file
func BitflyerWriteTransactions(ctx context.Context, outPath string, metadata *[]settlement.Transaction) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	if len(*metadata) == 0 {
		return nil
	}

	logger.Debug().Str("files", outPath).Int("num transactions", len(*metadata)).Msg("writing outputting files")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		logger.Error().Err(err).Msg("failed writing outputting files")
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

func setupSettlementTransactions(
	// token string,
	transactions map[string][]settlement.Transaction,
	limit decimal.Decimal,
	decimalFactor int32,
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
			last := 1+gwdIndex == len(groupedWithdrawals)
			if gwdIndex == 0 {
				aggregatedTx.AltCurrency = wd.AltCurrency
				aggregatedTx.Currency = wd.Currency
				aggregatedTx.Destination = wd.Destination
				aggregatedTx.Publisher = wd.Publisher
				aggregatedTx.WalletProvider = wd.WalletProvider
				aggregatedTx.WalletProviderID = wd.WalletProviderID
				aggregatedTx.Channel = wd.Channel
				aggregatedTx.SettlementID = wd.SettlementID
				aggregatedTx.Type = wd.Type
			}
			partialAmount := wd.Amount
			// will hit our limits
			if aggregatedTx.Amount.Add(partialAmount).GreaterThan(limit) {
				// reduce amount and fee to be within. can be zero
				partialAmount = limit.Sub(aggregatedTx.Amount)
			}
			// bitflyer constrains us to 8 decimal places
			tempTotal := aggregatedTx.Amount.Add(partialAmount)
			tempTotalTrunc := tempTotal.Truncate(decimalFactor)
			if last && tempTotal.GreaterThan(tempTotalTrunc) {
				// use reduced amount for last value
				partialAmount = partialAmount.Sub(tempTotal.Sub(tempTotalTrunc)) // do not truncate because only a part
			}
			// derive other number props
			partialProbi := altcurrency.BAT.ToProbi(partialAmount)            // scale to derive probi
			partialFee := wd.BATPlatformFee.Mul(partialAmount).Div(wd.Amount) // stoich to derive fee
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
			settlements[key] = append(settlements[key], wd)
		}
		j, _ := json.Marshal(aggregatedTx)
		fmt.Println(string(j))
		*set = append(*set, aggregatedTx)
		settlementRequests[index] = *set
	}
	return settlements, &settlementRequests, nil
}

func createBitflyerRequests(token string, settlementRequests *[][]settlement.Transaction) (*[]bitflyer.WithdrawToDepositIDBulkPayload, error) {
	bitflyerRequests := []bitflyer.WithdrawToDepositIDBulkPayload{}
	for _, withdrawalSet := range *settlementRequests {
		bitflyerPayloads, err := bitflyer.NewWithdrawsFromTxs("", &withdrawalSet) // self
		if err != nil {
			return nil, err
		}
		bitflyerRequests = append(bitflyerRequests, *bitflyer.NewWithdrawToDepositIDBulkPayload(
			os.Getenv("BITFLYER_DRYRUN") == "true",
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
	// settlementTransactions map[string]settlement.Transaction,
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

		var bitflyerBulkPayoutRequestRequirements SettlementRequest
		err = json.Unmarshal(bytes, &bitflyerBulkPayoutRequestRequirements)
		if err != nil {
			logger.Error().Err(err).Msg("failed unmarshal bulk payout file")
			return &submittedTransactions, err
		}
		// bat limit
		var decimalFactor int32 = 8
		limit := decimal.NewFromFloat32(200000). // start with jpy
								Div(quote.Rate).        // convert to bat
								Truncate(decimalFactor) // truncated to satoshis
		transactionsMap, transactionGroups, err := setupSettlementTransactions(
			bitflyerBulkPayoutRequestRequirements.Transactions,
			limit,
			decimalFactor,
		)
		if err != nil {
			return nil, err
		}

		requests, err := createBitflyerRequests(
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
					bitflyerBulkPayoutRequestRequirements.APIKey,
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
					bitflyerBulkPayoutRequestRequirements.APIKey,
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
