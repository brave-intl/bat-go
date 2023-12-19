package bitflyersettlement

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/settlement"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

var (
	notSubmittedCategory = "not-submitted"
)

// GroupSettlements groups settlements under a single wallet provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	settlements *[]custodian.Transaction,
) map[string][]custodian.Transaction {
	groupedByWalletProviderID := make(map[string][]custodian.Transaction)

	for _, payout := range *settlements {
		if payout.WalletProvider == "bitflyer" {
			walletProviderID := payout.WalletProviderID
			if groupedByWalletProviderID[walletProviderID] == nil {
				groupedByWalletProviderID[walletProviderID] = []custodian.Transaction{}
			}
			groupedByWalletProviderID[walletProviderID] = append(groupedByWalletProviderID[walletProviderID], payout)
		}
	}
	return groupedByWalletProviderID
}

// CategorizeResponse categorizes a response from bitflyer as pending, complete, failed, or unknown
func CategorizeResponse(
	batchByTransferID map[string]settlement.AggregateTransaction,
	payout *bitflyer.WithdrawToDepositIDResponse,
) ([]custodian.Transaction, string) {
	currentTx := batchByTransferID[payout.TransferID]
	key := payout.CategorizeStatus()

	currentTx.Status = key
	note := ""
	if payout.Message != "" {
		note = fmt.Sprintf("%s: %s. transferID: %s", payout.Status, payout.Message, payout.TransferID)
	} else {
		note = fmt.Sprintf("%s transferID: %s", payout.Status, payout.TransferID)
	}

	for i, tx := range currentTx.Inputs {
		// overwrite the amount
		tx.Note = note
		tmp := altcurrency.BAT
		tx.AltCurrency = &tmp
		tx.Currency = tmp.String()
		tx.Status = key
		currentTx.Inputs[i] = tx
	}
	return currentTx.Inputs, key
}

// CategorizeResponses categorizes the series of responses
func CategorizeResponses(
	batch []settlement.AggregateTransaction,
	response *[]bitflyer.WithdrawToDepositIDResponse,
) map[string][]custodian.Transaction {
	transactions := make(map[string][]custodian.Transaction)
	batchByTransferID := make(map[string]settlement.AggregateTransaction)

	for _, tx := range batch {
		batchByTransferID[tx.BitflyerTransferID()] = tx
	}

	for _, payout := range *response {
		inputs, key := CategorizeResponse(
			batchByTransferID,
			&payout,
		)
		transactions[key] = append(transactions[key], inputs...)
	}
	return transactions
}

// SubmitBulkPayoutTransactions submits bulk payout transactions
func SubmitBulkPayoutTransactions(
	ctx context.Context,
	batch []settlement.AggregateTransaction,
	submittedTransactions map[string][]custodian.Transaction,
	bulkPayoutRequestRequirements bitflyer.WithdrawToDepositIDBulkPayload,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]custodian.Transaction, error) {
	logger := logging.FromContext(ctx)
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
		batch,
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
	batch []settlement.AggregateTransaction,
	submittedTransactions map[string][]custodian.Transaction,
	bulkPayoutRequestRequirements bitflyer.WithdrawToDepositIDBulkPayload,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]custodian.Transaction, error) {
	logger := logging.FromContext(ctx)

	result, err := bitflyerClient.CheckPayoutStatus(
		ctx,
		bulkPayoutRequestRequirements.ToBulkStatus(),
	)
	if err != nil {
		return nil, err
	}
	response := CategorizeResponses(
		batch,
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
	transactionsByProviderID map[string][]custodian.Transaction,
	probiLimit decimal.Decimal,
	excludeLimited bool,
	sourceFrom string,
) (
	[]settlement.AggregateTransaction,
	[]custodian.Transaction,
	int,
	error,
) {
	// goes to bitflyer, does not include 0 value txs
	settlements := []settlement.AggregateTransaction{}
	// a list of settlements that are not being sent
	notSubmittedSettlements := []custodian.Transaction{}
	// number of transactions whose amounts were reduced
	numReduced := 0

	for _, groupedWithdrawals := range transactionsByProviderID {
		aggregatedTx := settlement.AggregateTransaction{}
		providerIDProbiLimit := probiLimit
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
				aggregatedTx.Inputs = []custodian.Transaction{}
			}
			partialProbi := limitedTx.Probi
			// will hit our limits
			if aggregatedTx.Probi.Add(partialProbi).GreaterThan(providerIDProbiLimit) {
				// reduce amount and fee to be within. can be zero
				if excludeLimited {
					// if we are excluding any limited transactions,
					// then simply reduce the limit for that bitflyer wallet
					providerIDProbiLimit = aggregatedTx.Probi
				} else {
					numReduced++
				}
				partialProbi = providerIDProbiLimit.Sub(aggregatedTx.Probi)
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
				aggregatedTx.Inputs = append(aggregatedTx.Inputs, limitedTx)
				// need separate so we can settle different types on eyeshade
				// update single settlement.
				limitedTx.Amount = partialAmount
				limitedTx.BATPlatformFee = partialFee
				limitedTx.Probi = partialProbi
			}
		}
		if !aggregatedTx.Probi.Equals(decimal.Zero) {
			//Bitflyer Specific requirement to truncate into 8 places or we will get API errors
			aggregatedTx.Probi = altcurrency.BAT.ToProbi(altcurrency.BAT.FromProbi(aggregatedTx.Probi).Truncate(8))
			aggregatedTx.SourceFrom = sourceFrom
			settlements = append(settlements, aggregatedTx)
		}
	}
	return settlements, notSubmittedSettlements, numReduced, nil
}

func CreateBitflyerRequest(
	dryRun *bitflyer.DryRunOption,
	token string,
	settlementRequests []settlement.AggregateTransaction,
) (*bitflyer.WithdrawToDepositIDBulkPayload, error) {
	set := []custodian.Transaction{}
	sourceFrom := ""
	for _, tx := range settlementRequests {
		set = append(set, tx.Transaction)
		sourceFrom = tx.SourceFrom
	}
	bitflyerPayloads, err := bitflyer.NewWithdrawsFromTxs(
		sourceFrom,
		set,
	)
	if err != nil {
		return nil, err
	}
	bitflyerRequest := bitflyer.NewWithdrawToDepositIDBulkPayload(
		dryRun,
		token,
		bitflyerPayloads,
	)
	return bitflyerRequest, nil
}

// PrepareRequests prepares requests
func PrepareRequests(
	ctx context.Context,
	bitflyerClient bitflyer.Client,
	txs []custodian.Transaction,
	excludeLimited bool,
	sourceFrom string,
) (*PreparedTransactions, error) {
	logger := logging.FromContext(ctx)

	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
	if err != nil {
		return nil, err
	}

	rate := quote.Rate

	// group by wallet provider id
	groupedByWalletProviderID := GroupSettlements(&txs)

	logger.Info().Int("count", len(groupedByWalletProviderID)).Msg("grouped bf transactions")

	// bat limit
	probiLimit := altcurrency.BAT.ToProbi(decimal.NewFromFloat32(200000). // start with jpy
										Div(rate).                      // convert to bat
										Mul(decimal.NewFromFloat(0.9)). // reduce by an extra 10% if we're paranoid
										Truncate(8))                    // truncated to satoshis
	transactions, notSubmittedTransactions, numReduced, err := setupSettlementTransactions(
		groupedByWalletProviderID,
		probiLimit,
		excludeLimited,
		sourceFrom,
	)
	if err != nil {
		return nil, err
	}

	jpyBATRate, _ := rate.Float64()

	transactionBatches := batchTransactions(ctx, transactions)

	logger.Info().Float64("JPY/BAT", jpyBATRate).Int("batches", len(transactionBatches)).Int("ignored", len(notSubmittedTransactions)).Int("reduced", numReduced).Msg("prepared bf transactions")

	return &PreparedTransactions{
		AggregateTransactionBatches: transactionBatches,
		NotSubmittedTransactions:    notSubmittedTransactions,
	}, nil
}

func batchTransactions(
	ctx context.Context,
	total []settlement.AggregateTransaction,
) [][]settlement.AggregateTransaction {
	vpr := viper.GetViper()
	chunkSize := float64(vpr.GetInt("chunk-size"))
	logger := logging.FromContext(ctx)
	chunked := [][]settlement.AggregateTransaction{}

	if chunkSize <= 1 {
		chunkSize = 10
	}

	inner := 0
	for _, agg := range total {
		inner += len(agg.Inputs)
	}

	length := float64(len(total))
	logger.Info().Float64("ChunkSize", chunkSize).Float64("Total", length).Int("inner count", inner).Msg("Chunking transactions")

	for i := float64(0); i < math.Ceil(length/chunkSize); i++ {
		start := i * chunkSize
		if start > length {
			break
		}
		end := start + chunkSize
		if end > length {
			end = length
		}

		chunked = append(chunked, total[int(start):int(end)])
	}

	logger.Info().Int("Chunks", len(chunked)).Msg("Chunked transactions")
	return chunked
}

// PreparedTransactions are the transactions which have been prepared into batches after applying limits, etc
type PreparedTransactions struct {
	// goes to bitflyer
	AggregateTransactionBatches [][]settlement.AggregateTransaction `json:"aggregateTransactionBatches"`
	// a list of settlements that are not being sent
	NotSubmittedTransactions []custodian.Transaction `json:"notSubmittedTransactions"`
}

// IterateRequest iterates requests
func IterateRequest(
	ctx context.Context,
	action string,
	bitflyerClient bitflyer.Client,
	prepared PreparedTransactions,
	dryRun *bitflyer.DryRunOption,
) (map[string][]custodian.Transaction, error) {
	logger := logging.FromContext(ctx)
	transactionBatches := prepared.AggregateTransactionBatches
	notSubmittedTransactions := prepared.NotSubmittedTransactions

	// for submission to eyeshade
	submittedTransactions := make(map[string][]custodian.Transaction)

	if len(notSubmittedTransactions) > 0 {
		submittedTransactions[notSubmittedCategory] = append(
			submittedTransactions[notSubmittedCategory],
			notSubmittedTransactions...,
		)
	}

	for i, batch := range transactionBatches {
		var totalValue decimal.Decimal = decimal.Zero
		for j, tx := range batch {
			tx.ProviderID = tx.BitflyerTransferID()
			batch[j] = tx
			totalValue = totalValue.Add(tx.Amount)
		}
		transactionBatches[i] = batch

		//  this will only fetch a new quote when needed - but ensures that we don't have problems due to quote expiring midway through
		quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
		if err != nil {
			return nil, err
		}

		request, err := CreateBitflyerRequest(
			dryRun,
			quote.PriceToken,
			batch,
		)
		if err != nil {
			return nil, err
		}

		if action == "upload" {
			inv, err := bitflyerClient.CheckInventory(ctx)
			if err != nil {
				return nil, err
			}
			threshold, err := decimal.NewFromString("0.9")
			if err != nil {
				return nil, err
			}
			logger.Info().Str("Required Funds", totalValue.String()).Str("available", inv["BAT"].Available.String()).Msg("Will continue if within threshold")
			if inv["BAT"].Available.Mul(threshold).LessThan(totalValue) {
				err = errors.New("not enough balance in account")
				logger.Error().Err(err).Msg("failed to submit bulk payout transactions due to insufficient available funds")
				return nil, err
			}

			submittedTransactions, err = SubmitBulkPayoutTransactions(
				ctx,
				transactionBatches[i],
				submittedTransactions,
				*request,
				bitflyerClient,
				len(transactionBatches),
				i+1,
			)
			if err != nil {
				logger.Error().Err(err).Msg("failed to submit bulk payout transactions")
				return nil, err
			}
		} else if action == "checkstatus" {
			submittedTransactions, err = CheckPayoutTransactionsStatus(
				ctx,
				transactionBatches[i],
				submittedTransactions,
				*request,
				bitflyerClient,
				len(transactionBatches),
				i+1,
			)
			if err != nil {
				logger.Error().Err(err).Msg("falied to check payout transactions status")
				return nil, err
			}
		}
	}

	return submittedTransactions, nil
}
