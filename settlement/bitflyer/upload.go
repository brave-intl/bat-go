package bitflyersettlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/shopspring/decimal"
)

var (
	notSubmittedCategory = "not-submitted"
)

// GroupSettlements groups settlements under a single provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	settlements *[]settlement.Transaction,
) map[string][]settlement.Transaction {
	byAttr := make(map[string]map[string]settlement.Transaction)
	byTransferID := make(map[string]settlement.Transaction)
	groupedByPublisher := make(map[string][]settlement.Transaction)
	channelsByPublisher := make(map[string][]string)

	for _, payout := range *settlements {
		if payout.WalletProvider == "bitflyer" {
			publisher := payout.Publisher
			channel := payout.Channel
			if byAttr[publisher] == nil {
				byAttr[publisher] = make(map[string]settlement.Transaction)
			}
			byAttr[publisher][channel] = payout
			byTransferID[payout.TransferID()] = payout
			channelsByPublisher[publisher] = append(channelsByPublisher[publisher], channel)
		}
	}
	for publisher, channels := range channelsByPublisher {
		sort.Strings(channels)
		for _, channel := range channels {
			groupedByPublisher[publisher] = append(groupedByPublisher[publisher], byAttr[publisher][channel])
		}
	}
	return groupedByPublisher
}

// CategorizeResponse categorizes a response from bitflyer as pending, complete, failed, or unknown
func CategorizeResponse(
	limitedSettlements map[string]settlement.Transaction,
	groupedByPublisher map[string][]settlement.Transaction,
	payout *bitflyer.WithdrawToDepositIDResponse,
) (settlement.Transaction, string) {
	currentTx := limitedSettlements[payout.TransferID]
	key := payout.CategorizeStatus()
	currentTx.Status = key
	note := ""
	if payout.Message != "" {
		note = fmt.Sprintf("%s: %s. transferID: %s", payout.Status, payout.Message, payout.TransferID)
	} else {
		note = fmt.Sprintf("%s transferID: %s", payout.Status, payout.TransferID)
	}
	// overwrite the amount
	currentTx.Note = note
	tmp := altcurrency.BAT
	currentTx.AltCurrency = &tmp
	currentTx.Currency = tmp.String()
	return currentTx, key
}

// CategorizeResponses categorizes the series of responses
func CategorizeResponses(
	limitedSettlements map[string]settlement.Transaction,
	groupedByPublisher map[string][]settlement.Transaction,
	response *[]bitflyer.WithdrawToDepositIDResponse,
) map[string][]settlement.Transaction {
	transactions := make(map[string][]settlement.Transaction)

	for _, payout := range *response {
		original, key := CategorizeResponse(
			limitedSettlements,
			groupedByPublisher,
			&payout,
		)
		transactions[key] = append(transactions[key], original)
	}
	return transactions
}

// SubmitBulkPayoutTransactions submits bulk payout transactions
func SubmitBulkPayoutTransactions(
	ctx context.Context,
	limitedSettlements map[string]settlement.Transaction,
	groupedByPublisher map[string][]settlement.Transaction,
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
	submitted := CategorizeResponses(
		limitedSettlements,
		groupedByPublisher,
		&response.Withdrawals,
	)
	for key, txs := range submitted {
		submittedTransactions[key] = append(submittedTransactions[key], txs...)
	}
	return submittedTransactions, nil
}

// CheckPayoutTransactionsStatus checks the status of given transactions
func CheckPayoutTransactionsStatus(
	ctx context.Context,
	limitedTransactions map[string]settlement.Transaction,
	groupedByPublisher map[string][]settlement.Transaction,
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
		bulkPayoutRequestRequirements.ToBulkStatus(),
	)
	if err != nil {
		return nil, err
	}
	response := CategorizeResponses(
		limitedTransactions,
		groupedByPublisher,
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
	transactionsByPublisher map[string][]settlement.Transaction,
	probiLimit decimal.Decimal,
	excludeLimited bool,
	// successfulPerPublisher map[string]int,
) (
	[]settlement.Transaction,
	[][]settlement.Transaction,
	[]settlement.Transaction,
	error,
) {
	// goes to bitflyer, does not include 0 value txs
	settlementRequests := [][]settlement.Transaction{}
	// goes to eyeshade, includes 0 value txs
	settlements := []settlement.Transaction{}
	// a list of settlements that are not being sent
	notSubmittedSettlements := []settlement.Transaction{}

	for _, groupedWithdrawals := range transactionsByPublisher {
		set, index := getSettlementGroup(settlementRequests, len(groupedWithdrawals))
		if index == len(settlementRequests) {
			settlementRequests = append(settlementRequests, set)
		}
		aggregatedTx := settlement.Transaction{}
		limitedTxs := []settlement.Transaction{}
		publisherProbiLimit := probiLimit
		for groupedWithdrawalIndex, limitedTx := range groupedWithdrawals {
			if groupedWithdrawalIndex == 0 {
				aggregatedTx.AltCurrency = limitedTx.AltCurrency
				aggregatedTx.Currency = limitedTx.Currency
				aggregatedTx.Destination = limitedTx.Destination
				aggregatedTx.Publisher = limitedTx.Publisher
				aggregatedTx.WalletProvider = limitedTx.WalletProvider
				aggregatedTx.WalletProviderID = limitedTx.WalletProviderID
				aggregatedTx.ProviderID = limitedTx.WalletProviderID
				aggregatedTx.Channel = limitedTx.Channel
				aggregatedTx.SettlementID = limitedTx.SettlementID
				aggregatedTx.Type = limitedTx.Type
			}
			partialProbi := limitedTx.Probi
			// will hit our limits
			if aggregatedTx.Probi.Add(partialProbi).GreaterThan(publisherProbiLimit) {
				// reduce amount and fee to be within. can be zero
				if excludeLimited {
					// if we are excluding any limited transactions,
					// then simply reduce the limit for that publisher
					publisherProbiLimit = aggregatedTx.Probi
				}
				partialProbi = publisherProbiLimit.Sub(aggregatedTx.Probi)
			}
			partialFee := decimal.Zero
			if limitedTx.BATPlatformFee.GreaterThan(decimal.Zero) {
				partialFee = partialProbi.Div(decimal.NewFromFloat(19)).Truncate(0)
			}
			// always in BAT to BAT so we're good
			partialAmount := altcurrency.BAT.FromProbi(partialProbi)
			// add to aggregate provider transaction
			aggregatedTx.Amount = aggregatedTx.Amount.Add(partialAmount)
			// not needed but useful for sanity checking
			aggregatedTx.BATPlatformFee = aggregatedTx.BATPlatformFee.Add(partialFee)
			aggregatedTx.Probi = aggregatedTx.Probi.Add(partialProbi)
			// attach to upper levels
			if partialProbi.Equals(decimal.Zero) {
				limitedTx.Status = "not-submitted"
				limitedTx.Note = "MONTHLY_SEND_LIMIT: not-submitted"
				notSubmittedSettlements = append(notSubmittedSettlements, limitedTx)
			} else {
				// need separate so we can settle different types on eyeshade
				// update single settlement.
				limitedTx.Amount = partialAmount
				limitedTx.BATPlatformFee = partialFee
				limitedTx.Probi = partialProbi
				limitedTxs = append(limitedTxs, limitedTx)
			}
		}
		settlements = append(settlements, limitedTxs...)
		set = append(set, aggregatedTx)
		settlementRequests[index] = set
	}
	return settlements, settlementRequests, notSubmittedSettlements, nil
}

func createBitflyerRequests(
	sourceFrom string,
	dryRun *bitflyer.DryRunOption,
	token string,
	settlementRequests [][]settlement.Transaction,
) (*[]bitflyer.WithdrawToDepositIDBulkPayload, error) {
	bitflyerRequests := []bitflyer.WithdrawToDepositIDBulkPayload{}
	for _, withdrawalSet := range settlementRequests {
		bitflyerPayloads, err := bitflyer.NewWithdrawsFromTxs(
			sourceFrom,
			withdrawalSet,
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
	settlementRequests [][]settlement.Transaction,
	toAdd int,
) ([]settlement.Transaction, int) {
	requestSeries := settlementRequests
	if len(requestSeries) == 0 {
		set := []settlement.Transaction{}
		return set, 0
	}
	lastIndex := len(requestSeries) - 1
	set := requestSeries[lastIndex]
	futureLength := len(requestSeries[lastIndex]) + toAdd
	if futureLength > 1000 {
		set := []settlement.Transaction{}
		return set, len(settlementRequests) - 1
	}
	return set, len(settlementRequests) - 1
}

// IterateRequest iterates requests
func IterateRequest(
	ctx context.Context,
	action string,
	bitflyerClient bitflyer.Client,
	bulkPayoutFiles []string,
	sourceFrom string,
	excludeLimited bool,
	dryRun *bitflyer.DryRunOption,
) (map[string][]settlement.Transaction, error) {

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	// for submission to eyeshade
	submittedTransactions := make(map[string][]settlement.Transaction)

	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
	if err != nil {
		return submittedTransactions, err
	}

	for _, bulkPayoutFile := range bulkPayoutFiles {
		bytes, err := ioutil.ReadFile(bulkPayoutFile)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read bulk payout file")
			return submittedTransactions, err
		}

		var txs []settlement.Transaction
		err = json.Unmarshal(bytes, &txs)
		if err != nil {
			logger.Error().Err(err).Msg("failed unmarshal bulk payout file")
			return submittedTransactions, err
		}

		for i, tx := range txs {
			tx.ProviderID = tx.TransferID()
			txs[i] = tx
		}
		// group by publisher and transfer id
		groupedByPublisher := GroupSettlements(&txs)
		// bat limit
		probiLimit := altcurrency.BAT.ToProbi(decimal.NewFromFloat32(200000). // start with jpy
											Div(quote.Rate). // convert to bat
			// Mul(decimal.NewFromFloat(0.9)). // reduce by an extra 10% if we're paranoid
			Truncate(8)) // truncated to satoshis
		limitedSettlements, transactionGroups, notSubmittedSettlements, err := setupSettlementTransactions(
			groupedByPublisher,
			probiLimit,
			excludeLimited,
		)
		if err != nil {
			return nil, err
		}
		if len(notSubmittedSettlements) > 0 {
			submittedTransactions[notSubmittedCategory] = append(
				submittedTransactions[notSubmittedCategory],
				notSubmittedSettlements...,
			)
		}

		limitedSettlementsByTransferID := mapSettlementsByTransferID(limitedSettlements)

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
					limitedSettlementsByTransferID,
					groupedByPublisher,
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
					limitedSettlementsByTransferID,
					groupedByPublisher,
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
	return submittedTransactions, nil
}

func mapSettlementsByTransferID(settlements []settlement.Transaction) map[string]settlement.Transaction {
	byTransferID := make(map[string]settlement.Transaction)
	for _, settlement := range settlements {
		byTransferID[settlement.TransferID()] = settlement
	}
	return byTransferID
}
