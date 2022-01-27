package bitflyersettlement

import (
	"context"
	"errors"
	"fmt"
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

// GroupSettlements groups settlements under a single wallet provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	settlements *[]settlement.Transaction,
) map[string][]settlement.Transaction {
	groupedByWalletProviderID := make(map[string][]settlement.Transaction)

	for _, payout := range *settlements {
		if payout.WalletProvider == "bitflyer" {
			walletProviderID := payout.WalletProviderID
			if groupedByWalletProviderID[walletProviderID] == nil {
				groupedByWalletProviderID[walletProviderID] = []settlement.Transaction{}
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
) ([]settlement.Transaction, string) {
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
) map[string][]settlement.Transaction {
	transactions := make(map[string][]settlement.Transaction)
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
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements bitflyer.WithdrawToDepositIDBulkPayload,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]settlement.Transaction, error) {
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
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements bitflyer.WithdrawToDepositIDBulkPayload,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]settlement.Transaction, error) {
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
	transactionsByProviderID map[string][]settlement.Transaction,
	probiLimit decimal.Decimal,
	excludeLimited bool,
) (
	[]settlement.Transaction,
	[][]settlement.AggregateTransaction,
	[]settlement.Transaction,
	int,
	error,
) {
	// goes to bitflyer, does not include 0 value txs
	settlementRequests := [][]settlement.AggregateTransaction{}
	// goes to eyeshade, includes 0 value txs
	settlements := []settlement.Transaction{}
	// a list of settlements that are not being sent
	notSubmittedSettlements := []settlement.Transaction{}
	// number of transactions whose amounts were reduced
	numReduced := 0

	for _, groupedWithdrawals := range transactionsByProviderID {
		set, index := getSettlementGroup(settlementRequests, len(groupedWithdrawals))
		if index == len(settlementRequests) {
			settlementRequests = append(settlementRequests, set)
		}
		aggregatedTx := settlement.AggregateTransaction{}
		limitedTxs := []settlement.Transaction{}
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
				aggregatedTx.Inputs = []settlement.Transaction{}
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
					numReduced += 1
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
				settlements = append(settlements, limitedTx)
				limitedTxs = append(limitedTxs, limitedTx)
			}
		}
		settlements = append(settlements, limitedTxs...)
		if !aggregatedTx.Probi.Equals(decimal.Zero) {
			set = append(set, aggregatedTx)
		}
		settlementRequests[index] = set
	}
	return settlements, settlementRequests, notSubmittedSettlements, numReduced, nil
}

func createBitflyerRequests(
	sourceFrom string,
	dryRun *bitflyer.DryRunOption,
	token string,
	settlementRequests [][]settlement.AggregateTransaction,
) (*[]bitflyer.WithdrawToDepositIDBulkPayload, error) {
	bitflyerRequests := []bitflyer.WithdrawToDepositIDBulkPayload{}
	for _, withdrawalSet := range settlementRequests {
		set := []settlement.Transaction{}
		for _, tx := range withdrawalSet {
			set = append(set, tx.Transaction)
		}
		bitflyerPayloads, err := bitflyer.NewWithdrawsFromTxs(
			sourceFrom,
			set,
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
	settlementRequests [][]settlement.AggregateTransaction,
	toAdd int,
) ([]settlement.AggregateTransaction, int) {
	requestSeries := settlementRequests
	if len(requestSeries) == 0 {
		set := []settlement.AggregateTransaction{}
		return set, 0
	}
	lastIndex := len(requestSeries) - 1
	set := requestSeries[lastIndex]
	futureLength := len(requestSeries[lastIndex]) + toAdd
	if futureLength > 1000 {
		set := []settlement.AggregateTransaction{}
		return set, len(settlementRequests) - 1
	}
	return set, len(settlementRequests) - 1
}

// PrepareRequests prepares requests
func PrepareRequests(
	ctx context.Context,
	bitflyerClient bitflyer.Client,
	txs []settlement.Transaction,
	excludeLimited bool,
) (*PreparedTransactions, error) {
	logger := logging.FromContext(ctx)

	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
	if err != nil {
		return nil, err
	}

	rate := quote.Rate

	// group by wallet provider id
	groupedByWalletProviderID := GroupSettlements(&txs)
	// bat limit
	probiLimit := altcurrency.BAT.ToProbi(decimal.NewFromFloat32(200000). // start with jpy
										Div(rate).                      // convert to bat
										Mul(decimal.NewFromFloat(0.9)). // reduce by an extra 10% if we're paranoid
										Truncate(8))                    // truncated to satoshis
	_, transactionBatches, notSubmittedTransactions, numReduced, err := setupSettlementTransactions(
		groupedByWalletProviderID,
		probiLimit,
		excludeLimited,
	)

	jpyBATRate, _ := rate.Float64()
	logger.Info().Float64("JPY/BAT", jpyBATRate).Int("batches", len(transactionBatches)).Int("ignored", len(notSubmittedTransactions)).Int("reduced", numReduced).Msg("prepared bf transactions")

	ptnx := breakOutTransactions(
		ctx,
		&PreparedTransactions{
			AggregateTransactionBatches: transactionBatches,
			NotSubmittedTransactions:    notSubmittedTransactions,
		})
	return ptnx, err
}

func breakOutTransactions(
	ctx context.Context,
	ptnx *PreparedTransactions,
) *PreparedTransactions {
	logger := logging.FromContext(ctx)
	var total []settlement.AggregateTransaction
	var chuncked [][]settlement.AggregateTransaction

	for i, _ := range ptnx.AggregateTransactionBatches {
		total = append(total, ptnx.AggregateTransactionBatches[i]...)
	}

	length := len(total)
	logger.Info().Int("Total Length", length).Msg("Chucnking transactions")
	chunkSize := 10
	for i := 0; i < length/chunkSize; i += 1 {
		start := i * chunkSize
		if start > length {
			break
		}
		end := start + chunkSize
		if end > length {
			end = length
		}
		currentChunk := total[start:end]
		chuncked[i] = make([]settlement.AggregateTransaction, len(currentChunk))
		chuncked[i] = currentChunk
	}

	logger.Info().Int("Chunks count", len(chuncked)).Msg("Chucnked transactions")
	ptnx.AggregateTransactionBatches = chuncked
	return ptnx
}

type PreparedTransactions struct {
	// goes to bitflyer
	AggregateTransactionBatches [][]settlement.AggregateTransaction `json:"aggregateTransactionBatches"`
	// a list of settlements that are not being sent
	NotSubmittedTransactions []settlement.Transaction `json:"notSubmittedTransactions"`
}

// IterateRequest iterates requests
func IterateRequest(
	ctx context.Context,
	action string,
	bitflyerClient bitflyer.Client,
	sourceFrom string,
	prepared PreparedTransactions,
	dryRun *bitflyer.DryRunOption,
) (map[string][]settlement.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	transactionBatches := prepared.AggregateTransactionBatches
	notSubmittedTransactions := prepared.NotSubmittedTransactions

	// for submission to eyeshade
	submittedTransactions := make(map[string][]settlement.Transaction)

	if len(notSubmittedTransactions) > 0 {
		submittedTransactions[notSubmittedCategory] = append(
			submittedTransactions[notSubmittedCategory],
			notSubmittedTransactions...,
		)
	}

	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
	if err != nil {
		return nil, err
	}
	for i, batch := range transactionBatches {
		for j, tx := range batch {
			tx.ProviderID = tx.BitflyerTransferID()
			batch[j] = tx
		}
		transactionBatches[i] = batch
	}

	requests, err := createBitflyerRequests(
		sourceFrom,
		dryRun,
		quote.PriceToken,
		transactionBatches,
	)
	if err != nil {
		return nil, err
	}

	if len(*requests) != len(transactionBatches) {
		return nil, errors.New("number of requests doesn't match number of batches!")
	}

	for i, request := range *requests {
		if action == "upload" {
			submittedTransactions, err = SubmitBulkPayoutTransactions(
				ctx,
				transactionBatches[i],
				submittedTransactions,
				request,
				bitflyerClient,
				len(*requests),
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
				request,
				bitflyerClient,
				len(*requests),
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
