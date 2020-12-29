package bitflyersettlement

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

// CategorizeResponse categorizes a response from bitflyer as pending, complete, failed, or unknown
func CategorizeResponse(
	originalTransactions map[string]settlement.Transaction,
	payout *bitflyer.PayoutResult,
) (settlement.Transaction, string) {
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

// GenerateTransferID generates a deterministic transaction reference id for idempotency
func GenerateTransferID(tx *settlement.Transaction) string {
	key := strings.Join([]string{
		tx.SettlementID,
		// if you have to resubmit referrals to get status
		tx.Type,
		tx.Destination,
		tx.Channel,
	}, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// CategorizeResponses categorizes the series of responses
func CategorizeResponses(
	originalTransactions map[string]settlement.Transaction,
	response *[]bitflyer.PayoutResult,
) map[string][]settlement.Transaction {
	transactions := make(map[string][]settlement.Transaction)

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
	transactionsMap map[string]settlement.Transaction,
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements bitflyer.PrivateRequestSequence,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
	signatureSwitch int,
) (map[string][]settlement.Transaction, error) {
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
		Str("api key", bulkPayoutRequestRequirements.APIKey).
		Str("signature", sig).
		Msg("sending request")

	response, err := bitflyerClient.UploadBulkPayout(
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
	transactionsMap map[string]settlement.Transaction,
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements bitflyer.PrivateRequestSequence,
	bitflyerClient bitflyer.Client,
	total int,
	blockProgress int,
) (map[string][]settlement.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	APIKey := bulkPayoutRequestRequirements.APIKey
	base := bulkPayoutRequestRequirements.Base
	clientID := base.OauthClientID
	for j, payout := range base.Payouts {
		result, err := bitflyerClient.CheckTxStatus(
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

// GeminiWriteTransactions writes settlement transactions to a json file
func GeminiWriteTransactions(ctx context.Context, outPath string, metadata *[]settlement.Transaction) error {
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

// GeminiWriteRequests writes settlement transactions to a json file
func GeminiWriteRequests(outPath string, metadata *[][]bitflyer.PayoutPayload) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

// ConvertTransactionsToPayouts converts transactions from antifraud to "payouts" for bitflyer
func ConvertTransactionsToPayouts(transactions *[]settlement.Transaction, txID string) (*[]bitflyer.PayoutPayload, decimal.Decimal) {
	payouts := make([]bitflyer.PayoutPayload, 0)
	total := decimal.NewFromFloat(0)
	for i, tx := range *transactions {
		tx.SettlementID = txID
		(*transactions)[i] = tx
		payout := bitflyer.SettlementTransactionToPayoutPayload(&tx)
		total = total.Add(payout.Amount)
		payouts = append(payouts, payout)
	}
	return &payouts, total
}

// TransformTransactions splits the transactions into appropriately sized blocks for signing
func TransformTransactions(ctx context.Context, oauthClientID string, transactions []settlement.Transaction) (*[][]bitflyer.PayoutPayload, error) {
	maxCount := 30
	blocksCount := (len(transactions) / maxCount) + 1
	privateRequests := make([][]bitflyer.PayoutPayload, 0)
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
	bitflyerClient bitflyer.Client,
	signatureSwitch int,
	bulkPayoutFiles []string,
	transactionsMap map[string]settlement.Transaction,
) (*map[string][]settlement.Transaction, error) {

	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	submittedTransactions := make(map[string][]settlement.Transaction)

	for _, bulkPayoutFile := range bulkPayoutFiles {
		bytes, err := ioutil.ReadFile(bulkPayoutFile)
		if err != nil {
			logger.Error().Err(err).Msg("failed to read bulk payout file")
			return &submittedTransactions, err
		}

		var bitflyerBulkPayoutRequestRequirements []bitflyer.PrivateRequestSequence
		err = json.Unmarshal(bytes, &bitflyerBulkPayoutRequestRequirements)
		if err != nil {
			logger.Error().Err(err).Msg("failed unmarshal bulk payout file")
			return &submittedTransactions, err
		}

		total := bitflyerComputeTotal(bitflyerBulkPayoutRequestRequirements)
		for i, bulkPayoutRequestRequirements := range bitflyerBulkPayoutRequestRequirements {
			blockProgress := bitflyerComputeTotal(bitflyerBulkPayoutRequestRequirements[:i+1])
			if action == "upload" {
				submittedTransactions, err = SubmitBulkPayoutTransactions(
					ctx,
					transactionsMap,
					submittedTransactions,
					bulkPayoutRequestRequirements,
					bitflyerClient,
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
					bitflyerClient,
					total,
					blockProgress,
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

func bitflyerComputeTotal(bitflyerBulkPayoutRequestRequirements []bitflyer.PrivateRequestSequence) int {
	if len(bitflyerBulkPayoutRequestRequirements) == 0 {
		return 0
	}

	firstLen := len(bitflyerBulkPayoutRequestRequirements[0].Base.Payouts)
	blockLen := len(bitflyerBulkPayoutRequestRequirements)
	lastLen := len(bitflyerBulkPayoutRequestRequirements[blockLen-1].Base.Payouts)
	total := blockLen * firstLen
	if blockLen > 1 {
		total += lastLen
	}
	return total
}
