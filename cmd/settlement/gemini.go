package settlement

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	oauthClientID string
	// uploading
	signatureSwitch     int
	allTransactionsFile string

	geminiSettlementCmd = &cobra.Command{
		Use:   "gemini",
		Short: "provides gemini settlement",
	}

	uploadGeminiSettlementCmd = &cobra.Command{
		Use:   "upload",
		Short: "uploads signed gemini transactions",
		Run: func(cmd *cobra.Command, args []string) {
<<<<<<< HEAD
<<<<<<< HEAD
			if err := GeminiUploadSettlement(
				cmd.Context(),
				viper.GetString("input"),
				signatureSwitch,
				allTransactionsFile,
				viper.GetString("out"),
			); err != nil {
=======
			if err := GeminiUploadSettlement(cmd.Context(), input, allTransactionsFile, out); err != nil {
>>>>>>> 1315533... updating loop
=======
			if err := GeminiUploadSettlement(cmd.Context(), "upload", input, signatureSwitch, allTransactionsFile, out); err != nil {
				logger, lerr := appctx.GetLogger(cmd.Context())
				if lerr != nil {
					_, logger = logging.SetupLogger(cmd.Context())
				}
				logger.Panic().Err(err).Msg("failed to perform upload to gemini")
			}
		},
	}

	checkStatusGeminiSettlementCmd = &cobra.Command{
		Use:   "checkstatus",
		Short: "uploads signed gemini transactions",
		Run: func(cmd *cobra.Command, args []string) {
			if err := GeminiUploadSettlement(cmd.Context(), "checkstatus", input, signatureSwitch, allTransactionsFile, out); err != nil {
>>>>>>> 91b9cb1... moved status check to individual endpoint
				logger, lerr := appctx.GetLogger(cmd.Context())
				if lerr != nil {
					_, logger = logging.SetupLogger(cmd.Context())
				}
				logger.Panic().Err(err).Msg("failed to perform upload to gemini")
			}
		},
	}

	transformGeminiSettlementCmd = &cobra.Command{
		Use:   "transform",
		Short: "provides transform of gemini settlement for mass pay",
		Run: func(cmd *cobra.Command, args []string) {
			if err := GeminiTransformForMassPay(
				cmd.Context(),
				viper.GetString("input"),
				oauthClientID,
				viper.GetString("out"),
			); err != nil {
				logger, lerr := appctx.GetLogger(cmd.Context())
				if lerr != nil {
					_, logger = logging.SetupLogger(cmd.Context())
				}
				logger.Panic().Err(err).Msg("failed to perform gemini transform")
			}
		},
	}
)

func init() {
	// add complete and transform subcommand
	geminiSettlementCmd.AddCommand(transformGeminiSettlementCmd)
	geminiSettlementCmd.AddCommand(uploadGeminiSettlementCmd)
	geminiSettlementCmd.AddCommand(checkStatusGeminiSettlementCmd)

	// add this command as a settlement subcommand
	SettlementCmd.AddCommand(geminiSettlementCmd)

	// setup the flags

	// input (required by all)
	geminiSettlementCmd.PersistentFlags().StringP("input", "i", "",
		"the file or comma delimited list of files that should be utilized")
	cmd.Must(viper.BindPFlag("input", geminiSettlementCmd.PersistentFlags().Lookup("input")))
	cmd.Must(viper.BindEnv("input", "INPUT"))
	cmd.Must(geminiSettlementCmd.MarkPersistentFlagRequired("input"))

	// out (required by all with default)
	geminiSettlementCmd.PersistentFlags().StringP("out", "o", "./gemini-settlement",
		"the location of the file")
	cmd.Must(viper.BindPFlag("out", geminiSettlementCmd.PersistentFlags().Lookup("out")))
	cmd.Must(viper.BindEnv("out", "OUT"))

	transformGeminiSettlementCmd.PersistentFlags().StringP("gemini-client-id", "g", "",
		"the oauth client id needed to check that the user authorized the payment")
	cmd.Must(viper.BindPFlag("gemini-client-id", geminiSettlementCmd.PersistentFlags().Lookup("gemini-client-id")))
	cmd.Must(viper.BindEnv("gemini-client-id", "GEMINI_CLIENT_ID"))
	cmd.Must(transformGeminiSettlementCmd.MarkPersistentFlagRequired("gemini-client-id"))
}

func categorizeResponse(
	originalTransactions map[string]settlement.Transaction,
	payout *gemini.PayoutResult,
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

func geminiSiftThroughResponses(
	originalTransactions map[string]settlement.Transaction,
	response *[]gemini.PayoutResult,
) map[string][]settlement.Transaction {
	transactions := make(map[string][]settlement.Transaction)

	for _, payout := range *response {
		original, key := categorizeResponse(
			originalTransactions,
			&payout,
		)
		transactions[key] = append(transactions[key], original)
	}
	return transactions
}

// GeminiUploadSettlement marks the settlement file as complete
func GeminiUploadSettlement(ctx context.Context, action string, inPath string, signatureSwitch int, allTransactionsFile string, outPath string) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	if outPath == "./gemini-settlement" {
		// use a file with extension if none is passed
		outPath = "./gemini-settlement-complete.json"
	}

	bulkPayoutFiles := strings.Split(inPath, ",")
	geminiClient, err := gemini.New()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create new gemini client")
		return err
	}

	if allTransactionsFile == "" {
		logger.Error().Msg("transactions file is empty")
		return errors.New("unable to upload without a transactions file to check against")
	}

	bytes, err := ioutil.ReadFile(allTransactionsFile)
	if err != nil {
		logger.Error().Err(err).Msg("failed to read the transactions file")
		return err
	}

	var settlementTransactions []settlement.AntifraudTransaction
	err = json.Unmarshal(bytes, &settlementTransactions)
	if err != nil {
		logger.Error().Err(err).Msg("failed to unmarshal the transactions file")
		return err
	}
	// create a map of the request transactions
	transactionsMap := geminiMapTransactionsToID(settlementTransactions)

	submittedTransactions, submitErr := geminiIterateRequest(ctx, action, geminiClient, signatureSwitch, bulkPayoutFiles, transactionsMap)
	// write file for upload to eyeshade
	logger.Info().
		Str("files", outPath).
		Msg("outputting files")

	for key, txs := range *submittedTransactions {
		outputPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-" + key + ".json"
		err = GeminiWriteTransactions(ctx, outputPath, &txs)
		if err != nil {
			logger.Error().Err(err).Msg("failed to write gemini transactions file")
			return err
		}
	}
	return submitErr
}

func geminiIterateRequest(
	ctx context.Context,
	action string,
	geminiClient gemini.Client,
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

		var geminiBulkPayoutRequestRequirements []gemini.PrivateRequestSequence
		err = json.Unmarshal(bytes, &geminiBulkPayoutRequestRequirements)
		if err != nil {
			logger.Error().Err(err).Msg("failed unmarshal bulk payout file")
			return &submittedTransactions, err
		}

		total := geminiComputeTotal(geminiBulkPayoutRequestRequirements)
		for i, bulkPayoutRequestRequirements := range geminiBulkPayoutRequestRequirements {
			if action == "upload" {
				submittedTransactions, err = submitBulkPayoutTransactions(
					ctx,
					transactionsMap,
					submittedTransactions,
					bulkPayoutRequestRequirements,
					geminiClient,
					len(bulkPayoutFiles),
					signatureSwitch,
					i,
				)
			} else if action == "checkstatus" {
				submittedTransactions, err = checkPayoutTransactionsStatus(
					ctx,
					transactionsMap,
					submittedTransactions,
					bulkPayoutRequestRequirements,
					geminiClient,
					total,
					i,
				)
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return &submittedTransactions, nil
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

func checkPayoutTransactionsStatus(
	ctx context.Context,
	transactionsMap map[string]settlement.Transaction,
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements gemini.PrivateRequestSequence,
	geminiClient gemini.Client,
	total int,
	i int,
) (map[string][]settlement.Transaction, error) {
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
		original, key := categorizeResponse(
			transactionsMap,
			result,
		)
		submittedTransactions[key] = append(submittedTransactions[key], original)

		logger.Debug().
			Int("total", total).
			Int("block", i).
			Int("payout", j).
			Str("key", key).
			Str("tx_ref", payout.TxRef).
			Msg("parameters used")
	}
	return submittedTransactions, err
}

func submitBulkPayoutTransactions(
	ctx context.Context,
	transactionsMap map[string]settlement.Transaction,
	submittedTransactions map[string][]settlement.Transaction,
	bulkPayoutRequestRequirements gemini.PrivateRequestSequence,
	geminiClient gemini.Client,
	total int,
	signatureSwitch int,
	i int,
) (map[string][]settlement.Transaction, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	logging.SubmitProgress(ctx, i, total)
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
		Int("i", i).
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
	submitted := geminiSiftThroughResponses(transactionsMap, response)
	for key, txs := range submitted {
		submittedTransactions[key] = append(submittedTransactions[key], txs...)
	}
	return submittedTransactions, nil
}

// geminiMapTransactionsToID creates a map of guid's to transactions
func geminiMapTransactionsToID(transactions []settlement.AntifraudTransaction) map[string]settlement.Transaction {
	transactionsMap := make(map[string]settlement.Transaction)
	for _, atx := range transactions {
		tx := atx.ToTransaction()
		transactionsMap[gemini.GenerateTxRef(&tx)] = tx
	}
	return transactionsMap
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
func GeminiWriteRequests(outPath string, metadata *[][]gemini.PayoutPayload) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

// GeminiTransformForMassPay starts the process to transform a settlement into a mass pay csv
func GeminiTransformForMassPay(ctx context.Context, input string, oauthClientID string, output string) (err error) {
	transactions, err := settlement.ReadFiles(strings.Split(input, ","))
	if err != nil {
		return err
	}

	geminiPayouts, err := GeminiTransformTransactions(ctx, oauthClientID, *transactions)
	if err != nil {
		return err
	}
	err = GeminiWriteRequests(output, geminiPayouts)
	if err != nil {
		return err
	}
	return nil
}

// GeminiConvertTransactionsToGeminiPayouts converts transactions from antifraud to "payouts" for gemini
func GeminiConvertTransactionsToGeminiPayouts(transactions *[]settlement.Transaction, txID string) (*[]gemini.PayoutPayload, decimal.Decimal) {
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

// GeminiTransformTransactions splits the transactions into appropriately sized blocks for signing
func GeminiTransformTransactions(ctx context.Context, oauthClientID string, transactions []settlement.Transaction) (*[][]gemini.PayoutPayload, error) {
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
			payoutBlock, blockTotal := GeminiConvertTransactionsToGeminiPayouts(&transactionBlock, txnID)
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
