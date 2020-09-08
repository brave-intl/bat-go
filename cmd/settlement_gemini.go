package cmd

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
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
			if err := GeminiUploadSettlement(input, signatureSwitch, allTransactionsFile, out); err != nil {
				fmt.Printf("failed to perform upload to gemini: %s\n", err)
				os.Exit(1)
			}
		},
	}

	transformGeminiSettlementCmd = &cobra.Command{
		Use:   "transform",
		Short: "provides transform of gemini settlement for mass pay",
		Run: func(cmd *cobra.Command, args []string) {
			if err := GeminiTransformForMassPay(input, oauthClientID, out); err != nil {
				log.Printf("failed to perform transform: %s\n", err)
				os.Exit(1)
			}
		},
	}
)

func init() {
	// add complete and transform subcommand
	geminiSettlementCmd.AddCommand(transformGeminiSettlementCmd)
	geminiSettlementCmd.AddCommand(uploadGeminiSettlementCmd)

	// add this command as a settlement subcommand
	settlementCmd.AddCommand(geminiSettlementCmd)

	// setup the flags

	// input (required by all)
	geminiSettlementCmd.PersistentFlags().StringVarP(&input, "input", "i", "",
		"the file or comma delimited list of files that should be utilized")
	must(viper.BindPFlag("input", geminiSettlementCmd.PersistentFlags().Lookup("input")))
	must(viper.BindEnv("input", "INPUT"))
	must(geminiSettlementCmd.MarkPersistentFlagRequired("input"))

	// out (required by all with default)
	geminiSettlementCmd.PersistentFlags().StringVarP(&out, "out", "o", "./gemini-settlement",
		"the location of the file")
	must(viper.BindPFlag("out", geminiSettlementCmd.PersistentFlags().Lookup("out")))
	must(viper.BindEnv("out", "OUT"))

	// txnID (required by transform)
	transformGeminiSettlementCmd.PersistentFlags().StringVarP(&txnID, "txn-id", "t", "",
		"the completed mass pay transaction id")
	must(viper.BindPFlag("txn-id", geminiSettlementCmd.PersistentFlags().Lookup("txn-id")))
	must(viper.BindEnv("txn-id", "TXN_ID"))
	must(transformGeminiSettlementCmd.MarkPersistentFlagRequired("txn-id"))

	transformGeminiSettlementCmd.PersistentFlags().StringVarP(&oauthClientID, "gemini-client-id", "g", "",
		"the oauth client id needed to check that the user authorized the payment")
	must(viper.BindPFlag("gemini-client-id", geminiSettlementCmd.PersistentFlags().Lookup("gemini-client-id")))
	must(viper.BindEnv("gemini-client-id", "GEMINI_CLIENT_ID"))
	must(transformGeminiSettlementCmd.MarkPersistentFlagRequired("gemini-client-id"))
}

func geminiSiftThroughResponses(
	originalTransactions map[string]settlement.Transaction,
	response *[]gemini.PayoutResult,
) map[string][]settlement.Transaction {
	transactions := make(map[string][]settlement.Transaction)

	for _, payout := range *response {
		if payout.Result == "Error" {
			transactions["failed"] = append(transactions["failed"], originalTransactions[payout.TxRef])
		} else {
			if *payout.Status == "Pending" {
				transactions["pending"] = append(transactions["pending"], originalTransactions[payout.TxRef])
			} else {
				transactions["completed"] = append(transactions["completed"], originalTransactions[payout.TxRef])
			}
		}
	}
	return transactions
}

func setupLogger(ctx context.Context) (context.Context, *zerolog.Logger) {
	return logging.SetupLogger(context.WithValue(ctx, appctx.EnvironmentCTXKey, os.Getenv("ENV")))
}

// GeminiUploadSettlement marks the settlement file as complete
func GeminiUploadSettlement(inPath string, signatureSwitch int, allTransactionsFile string, outPath string) error {
	if outPath == "./gemini-settlement" {
		// use a file with extension if none is passed
		outPath = "./gemini-settlement-complete.json"
	}

	ctx, _ := setupLogger(context.Background())
	bulkPayoutFiles := strings.Split(inPath, ",")
	geminiClient, err := gemini.New()
	if err != nil {
		return err
	}

	if allTransactionsFile == "" {
		return errors.New("unable to upload without a transactions file to check against")
	}
	submittedTransactions := make(map[string][]settlement.Transaction)

	bytes, err := ioutil.ReadFile(allTransactionsFile)
	if err != nil {
		return err
	}

	var settlementTransactions []settlement.AntifraudTransaction
	err = json.Unmarshal(bytes, &settlementTransactions)
	if err != nil {
		return err
	}
	// create a map of the request transactions
	transactionsMap := geminiMapTransactionsToID(settlementTransactions)

	for _, bulkPayoutFile := range bulkPayoutFiles {
		bytes, err := ioutil.ReadFile(bulkPayoutFile)
		if err != nil {
			return err
		}

		var geminiBulkPayoutRequestRequirements []gemini.PrivateRequestSequence
		err = json.Unmarshal(bytes, &geminiBulkPayoutRequestRequirements)
		if err != nil {
			return err
		}

		for _, bulkPayoutRequestRequirements := range geminiBulkPayoutRequestRequirements {
			// make sure payload is parsable
			// upload the bulk payout
			decodedSig, err := hex.DecodeString(bulkPayoutRequestRequirements.Signatures[signatureSwitch])
			if err != nil {
				return err
			}
			presigner := cryptography.NewPresigner(decodedSig)
			bulkPayoutRequestRequirements.Base.Nonce = bulkPayoutRequestRequirements.Base.Nonce + int64(signatureSwitch)
			serialized, err := json.Marshal(bulkPayoutRequestRequirements.Base)
			if err != nil {
				return err
			}
			payload := base64.StdEncoding.EncodeToString(serialized)
			response, err := geminiClient.UploadBulkPayout(
				ctx,
				bulkPayoutRequestRequirements.APIKey,
				presigner,
				payload,
			)
			if err != nil {
				return err
			}
			// collect all successful transactions to send to eyeshade
			submitted := geminiSiftThroughResponses(transactionsMap, response)
			for key, txs := range submitted {
				submittedTransactions[key] = append(submittedTransactions[key], txs...)
			}
		}
	}
	// write file for upload to eyeshade
	for key, txs := range submittedTransactions {
		outputPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-" + key + ".json"
		err = GeminiWriteTransactions(outputPath, &txs)
		if err != nil {
			return err
		}
	}
	return nil
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
func GeminiWriteTransactions(outPath string, metadata *[]settlement.Transaction) error {
	if len(*metadata) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
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
func GeminiTransformForMassPay(input string, oauthClientID string, output string) (err error) {
	transactions, err := settlement.ReadFiles(strings.Split(input, ","))
	if err != nil {
		return err
	}

	geminiPayouts, err := GeminiTransformTransactions(oauthClientID, *transactions)
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
func GeminiTransformTransactions(oauthClientID string, transactions []settlement.Transaction) (*[][]gemini.PayoutPayload, error) {
	maxCount := 500
	blocksCount := (len(transactions) / maxCount) + 1
	privateRequests := make([][]gemini.PayoutPayload, 0)
	i := 0

	txnID := transactions[0].SettlementID
	txID := uuid.Must(uuid.FromString(txnID))
	fmt.Println("transaction id", txID.String())

	fmt.Printf("creating %d blocks\n", blocksCount)
	fmt.Printf("with %d transactions\n", len(transactions))
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
	fmt.Printf("%s bat to be paid out\n", total.String())
	return &privateRequests, nil
}
