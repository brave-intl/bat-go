package geminisettlement

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// CategorizeResponse categorizes a response from gemini as pending, complete, failed, or unknown
func CategorizeResponse(
	originalTransactions map[string]custodian.Transaction,
	payout *gemini.PayoutResult,
) (custodian.Transaction, string) {
	original := originalTransactions[payout.TxRef]
	key := "failed"
	if payout.Result == "Error" {
		original.Note = *payout.Reason
	} else {
		status := *payout.Status
		key = "unknown"
		if *payout.Status == "Pending" {
			key = "pending"
		} else if status == "Completed" {
			key = "complete"
		}
	}
	original.Status = key
	tmp := altcurrency.BAT
	original.AltCurrency = &tmp
	original.Currency = tmp.String()
	original.ProviderID = payout.TxRef
	return original, key
}

// CategorizeResponses categorizes the series of responses
func CategorizeResponses(
	originalTransactions map[string]custodian.Transaction,
	response *[]gemini.PayoutResult,
) map[string][]custodian.Transaction {
	transactions := make(map[string][]custodian.Transaction)

	for _, payout := range *response {
		original, key := CategorizeResponse(
			originalTransactions,
			&payout,
		)
		transactions[key] = append(transactions[key], original)
	}
	return transactions
}

// SubmitBulkPayoutTransactions submits bulk payout transactions
func SubmitBulkPayoutTransactions(
	ctx context.Context,
	transactionsMap map[string]custodian.Transaction,
	submittedTransactions map[string][]custodian.Transaction,
	bulkPayoutRequestRequirements gemini.PrivateRequestSequence,
	geminiClient gemini.Client,
	total int,
	blockProgress int,
	signatureSwitch int,
) (map[string][]custodian.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	logging.SubmitProgress(ctx, blockProgress, total)
	// make sure payload is parsable
	// upload the bulk payout
	sig := bulkPayoutRequestRequirements.Signatures[signatureSwitch]
	decodedSig, err := hex.DecodeString(sig)
	if err != nil {
		logger.Error().Err(err).Msg("failed decode signature")
		return submittedTransactions, err
	}
	base := bulkPayoutRequestRequirements.Base
	presigner := cryptography.NewPresigner(decodedSig)
	base.Nonce = base.Nonce + int64(signatureSwitch)

	logger.Debug().
		Int("total", total).
		Int("progress", blockProgress).
		Int64("nonce", base.Nonce).
		Int("signature switch", signatureSwitch).
		Msg("parameters used")

	serialized, err := json.Marshal(base)
	if err != nil {
		return submittedTransactions, err
	}
	payload := base64.StdEncoding.EncodeToString(serialized)

	logger.Debug().
		Str("api_key", bulkPayoutRequestRequirements.APIKey).
		Str("signature", sig).
		Msg("sending request")

	response, err := geminiClient.UploadBulkPayout(
		ctx,
		bulkPayoutRequestRequirements.APIKey,
		presigner,
		payload,
	)
	<-time.After(time.Second)
	if err != nil {
		logger.Error().Err(err).Msg("error performing upload")
		return submittedTransactions, err
	}
	// collect all successful transactions to send to eyeshade
	submitted := CategorizeResponses(transactionsMap, response)
	for key, txs := range submitted {
		submittedTransactions[key] = append(submittedTransactions[key], txs...)
	}
	return submittedTransactions, nil
}

// CheckPayoutTransactionsStatus checks the status of given transactions
func CheckPayoutTransactionsStatus(
	ctx context.Context,
	transactionsMap map[string]custodian.Transaction,
	submittedTransactions map[string][]custodian.Transaction,
	bulkPayoutRequestRequirements gemini.PrivateRequestSequence,
	geminiClient gemini.Client,
	total int,
	blockProgress int,
) (map[string][]custodian.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	APIKey := bulkPayoutRequestRequirements.APIKey
	base := bulkPayoutRequestRequirements.Base
	clientID := base.OauthClientID
	for j, payout := range base.Payouts {
		result, err := geminiClient.CheckTxStatus(
			ctx,
			APIKey,
			clientID,
			payout.TxRef,
		)
		if err != nil {
			return nil, err
		}
		original, key := CategorizeResponse(
			transactionsMap,
			result,
		)
		submittedTransactions[key] = append(submittedTransactions[key], original)

		prog := blockProgress + j
		logging.SubmitProgress(ctx, prog, total)
		logger.Debug().
			Int("total", total).
			Int("progress", prog).
			Str("key", key).
			Str("tx_ref", payout.TxRef).
			Msg("parameters used")
	}
	return submittedTransactions, err
}

// ConvertTransactionsToPayouts converts transactions from antifraud to "payouts" for gemini
func ConvertTransactionsToPayouts(transactions *[]custodian.Transaction, txID string) (*[]gemini.PayoutPayload, decimal.Decimal) {
	payouts := make([]gemini.PayoutPayload, 0)
	total := decimal.NewFromFloat(0)
	for i, tx := range *transactions {
		tx.SettlementID = txID
		(*transactions)[i] = tx
		payout := gemini.SettlementTransactionToPayoutPayload(&tx)
		total = total.Add(payout.Amount)
		payouts = append(payouts, payout)
	}
	return &payouts, total
}

// TransformTransactions splits the transactions into appropriately sized blocks for signing
func TransformTransactions(ctx context.Context, oauthClientID string, transactions []custodian.Transaction) (*[][]gemini.PayoutPayload, error) {
	maxCount := 30
	blocksCount := (len(transactions) / maxCount) + 1
	privateRequests := make([][]gemini.PayoutPayload, 0)
	i := 0
	logger := zerolog.Ctx(ctx)

	txnID := transactions[0].SettlementID
	txID := uuid.Must(uuid.FromString(txnID))
	total := decimal.NewFromFloat(0)
	for i < blocksCount {
		lowerBound := i * maxCount
		upperBound := (i + 1) * maxCount
		payoutLength := len(transactions)
		if payoutLength <= upperBound {
			upperBound = payoutLength
		}
		transactionBlock := transactions[lowerBound:upperBound]
		if len(transactionBlock) > 0 {
			payoutBlock, blockTotal := ConvertTransactionsToPayouts(&transactionBlock, txnID)
			total = total.Add(blockTotal)
			privateRequests = append(privateRequests, *payoutBlock)
		}
		i++
	}

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("transaction_id", txID.String()).
			Int("blocks", blocksCount).
			Int("transactions", len(transactions)).
			Str("total", total.String())
	})

	return &privateRequests, nil
}

// IterateRequest iterates requests
func IterateRequest(
	ctx context.Context,
	action string,
	geminiClient gemini.Client,
	signatureSwitch int,
	bulkPayoutFiles []string,
	transactionsMap map[string]custodian.Transaction,
) (map[string][]custodian.Transaction, error) {

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	submittedTransactions := make(map[string][]custodian.Transaction)

	apiSecret, err := appctx.GetStringFromContext(ctx, appctx.GeminiAPISecretCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get gemini api secret")
		return submittedTransactions, fmt.Errorf("failed to get gemini api secret: %w", err)
	}
	apiKey, err := appctx.GetStringFromContext(ctx, appctx.GeminiAPIKeyCTXKey)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get gemini api key")
		return submittedTransactions, fmt.Errorf("failed to get gemini api key: %w", err)
	}

	for _, bulkPayoutFile := range bulkPayoutFiles {
		bytes, err := ioutil.ReadFile(bulkPayoutFile)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read bulk payout file")
			return submittedTransactions, err
		}

		var geminiBulkPayoutRequestRequirements []gemini.PrivateRequestSequence
		err = json.Unmarshal(bytes, &geminiBulkPayoutRequestRequirements)
		if err != nil {
			logger.Error().Err(err).Msg("failed unmarshal bulk payout file")
			return submittedTransactions, err
		}

		total := geminiComputeTotal(geminiBulkPayoutRequestRequirements)
		for i, bulkPayoutRequestRequirements := range geminiBulkPayoutRequestRequirements {
			blockProgress := geminiComputeTotal(geminiBulkPayoutRequestRequirements[:i+1])
			if action == "upload" {
				payload, err := json.Marshal(gemini.NewBalancesPayload(nil))
				if err != nil {
					logger.Error().Err(err).Msg("failed unmarshal balance payload")
					return submittedTransactions, err
				}

				signer := cryptography.NewHMACHasher([]byte(apiSecret))
				result, err := geminiClient.FetchBalances(ctx, apiKey, signer, string(payload))
				availableCurrency := map[string]decimal.Decimal{}
				for _, currency := range *result {
					availableCurrency[currency.Currency] = currency.Amount
				}

				requiredCurrency := map[string]decimal.Decimal{}
				for _, pay := range bulkPayoutRequestRequirements.Base.Payouts {
					requiredCurrency[pay.Currency] = requiredCurrency[pay.Currency].Add(pay.Amount)
				}

				for key, amount := range requiredCurrency {
					if availableCurrency[key].LessThan(amount) {
						logger.Error().Str("required", amount.String()).Str("available", availableCurrency[key].String()).Str("currency", key).Err(err).Msg("failed to meet required balance")
						return submittedTransactions, fmt.Errorf("failed to meet required balance: %w", err)
					}
				}

				submittedTransactions, err = SubmitBulkPayoutTransactions(
					ctx,
					transactionsMap,
					submittedTransactions,
					bulkPayoutRequestRequirements,
					geminiClient,
					len(bulkPayoutFiles),
					blockProgress,
					signatureSwitch,
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
					bulkPayoutRequestRequirements,
					geminiClient,
					total,
					blockProgress,
				)
				if err != nil {
					logger.Error().Err(err).Msg("failed to check payout transactions status")
					return nil, err
				}
			}
		}
	}
	return submittedTransactions, nil
}

func geminiComputeTotal(geminiBulkPayoutRequestRequirements []gemini.PrivateRequestSequence) int {
	if len(geminiBulkPayoutRequestRequirements) == 0 {
		return 0
	}

	firstLen := len(geminiBulkPayoutRequestRequirements[0].Base.Payouts)
	blockLen := len(geminiBulkPayoutRequestRequirements)
	lastLen := len(geminiBulkPayoutRequestRequirements[blockLen-1].Base.Payouts)
	total := blockLen * firstLen
	if blockLen > 1 {
		total += lastLen
	}
	return total
}
