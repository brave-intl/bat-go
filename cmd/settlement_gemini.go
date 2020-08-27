package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	geminiSettlementCmd = &cobra.Command{
		Use:   "gemini",
		Short: "provides gemini settlement",
	}

	uploadGeminiSettlementCmd = &cobra.Command{
		Use:   "upload",
		Short: "uploads signed gemini transactions",
		Run: func(cmd *cobra.Command, args []string) {
			if err := GeminiUploadSettlement(input, out); err != nil {
				log.Printf("failed to perform upload to gemini: %s\n", err)
				os.Exit(1)
			}
		},
	}

	transformGeminiSettlementCmd = &cobra.Command{
		Use:   "transform",
		Short: "provides transform of gemini settlement for mass pay",
		// PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// 	// add flag values to our base context that need to be there
		// 	ctx = context.WithValue(ctx, appctx.RatiosServerCTXKey, viper.Get("ratios-service"))
		// 	ctx = context.WithValue(ctx, appctx.RatiosAccessTokenCTXKey, viper.Get("ratios-token"))
		// },
		Run: func(cmd *cobra.Command, args []string) {
			if err := GeminiTransformForMassPay(input, out); err != nil {
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
}

// func geminiValidateResponse(
// 	transactions *[]gemini.PayoutPayload,
// 	response *[]gemini.PayoutResult,
// ) (map[string]gemini.PayoutPayload, error) {
// 	if len(*transactions) != len(*response) {
// 		return nil, errors.New("response count did not match request count")
// 	}
// 	mappedTransactions := convertTransactionListIntoMap(transactions)
// 	return mappedTransactions, nil
// }

func geminiSiftThroughResponses(
	originalTransactions map[string]settlement.Transaction,
	response *[]gemini.PayoutResult,
) []settlement.Transaction {
	successful := []settlement.Transaction{}

	for _, payout := range *response {
		if payout.Result == "Error" {
			// fmt.Println(payout.GenerateLog())
		} else {
			successful = append(successful, originalTransactions[payout.TxRef])
		}
	}
	return successful
}

// func convertTransactionListIntoMap(
// 	transactions *[]gemini.PayoutPayload,
// ) map[string]gemini.PayoutPayload {
// 	transactionMap := make(map[string]gemini.PayoutPayload)
// 	for _, payoutRequest := range *transactions {
// 		transactionMap[payoutRequest.TxRef] = payoutRequest
// 	}
// 	return transactionMap
// }

// GeminiUploadSettlement marks the settlement file as complete
func GeminiUploadSettlement(inPath string, outPath string) error {
	if outPath == "./gemini-settlement" {
		// use a file with extension if none is passed
		outPath = "./gemini-settlement-complete.json"
	}

	ctx := context.Background()
	bulkPayoutFiles := strings.Split(inPath, ",")
	geminiClient, err := gemini.New()
	if err != nil {
		return err
	}

	transactionsForEyeshade := []settlement.Transaction{}
	for _, bulkPayoutFile := range bulkPayoutFiles {
		bytes, err := ioutil.ReadFile(bulkPayoutFile)
		if err != nil {
			return err
		}

		var geminiBulkPayoutRequestRequirements []gemini.PrivateRequest
		err = json.Unmarshal(bytes, &geminiBulkPayoutRequestRequirements)
		if err != nil {
			return err
		}
		for _, bulkPayoutRequestRequirements := range geminiBulkPayoutRequestRequirements {
			// make sure payload is parsable
			// upload the bulk payout
			fmt.Printf("%#v\n", bulkPayoutRequestRequirements)
			response, err := geminiClient.UploadBulkPayout(ctx, bulkPayoutRequestRequirements)
			if err != nil {
				return err
			}
			// // create a map of the request transactions
			transactionsMap := geminiMapTransactionsToID(bulkPayoutRequestRequirements.Transactions)
			// collect all successful transactions to send to eyeshade
			transactionsForEyeshade = append(
				transactionsForEyeshade,
				geminiSiftThroughResponses(transactionsMap, response)...,
			)
		}
	}
	// write file for upload to eyeshade
	err = GeminiWriteTransactions(outPath, &transactionsForEyeshade)
	if err != nil {
		return err
	}
	return nil
}

// geminiMapTransactionsToID creates a map of guid's to transactions
func geminiMapTransactionsToID(transactions []settlement.Transaction) map[string]settlement.Transaction {
	transactionsMap := make(map[string]settlement.Transaction)
	for _, tx := range transactions {
		transactionsMap[gemini.GenerateTxRef(&tx)] = tx
	}
	return transactionsMap
}

// GeminiWriteTransactions writes settlement transactions to a json file
func GeminiWriteTransactions(outPath string, metadata *[]settlement.Transaction) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

// GeminiWriteRequests writes settlement transactions to a json file
func GeminiWriteRequests(outPath string, metadata *[]gemini.PrivateRequest) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}

// GeminiTransformForMassPay starts the process to transform a settlement into a mass pay csv
func GeminiTransformForMassPay(input string, output string) (err error) {
	transactions, err := settlement.ReadFiles(strings.Split(input, ","))
	if err != nil {
		return err
	}

	geminiPayouts, err := GeminiTransformTransactions(*transactions)
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
func GeminiTransformTransactions(transactions []settlement.Transaction) (*[]gemini.PrivateRequest, error) {
	maxCount := 500
	blocksCount := (len(transactions) / maxCount) + 1
	privateRequests := make([]gemini.PrivateRequest, 0)
	i := 0

	txnID := transactions[0].SettlementID
	txID := uuid.Must(uuid.FromString(txnID))
	fmt.Println("transaction id", txID.String())

	fmt.Printf("creating %d blocks\n", blocksCount)
	fmt.Printf("with %d transactions\n", len(transactions))
	total := decimal.NewFromFloat(0)
	for i < blocksCount {
		var transactionBlock []settlement.Transaction
		lowerBound := i * maxCount
		upperBound := (i + 1) * maxCount
		payoutLength := len(transactions)
		if payoutLength <= upperBound {
			upperBound = payoutLength
		}
		transactionBlock = transactions[lowerBound:upperBound]
		payoutBlock, blockTotal := GeminiConvertTransactionsToGeminiPayouts(&transactionBlock, txnID)
		total = total.Add(blockTotal)
		// marshal the payout block
		bulkPaymentPayloadSerialized, err := json.Marshal(gemini.NewBulkPayoutPayload(payoutBlock))
		if err != nil {
			return nil, err
		}
		// create space for the gemini request to be signed offline
		privateRequest := gemini.PrivateRequest{
			Payload:      string(bulkPaymentPayloadSerialized),
			Transactions: transactionBlock,
		}
		// append to list of requests to make in future
		privateRequests = append(privateRequests, privateRequest)
		// increment i
		i++
	}
	fmt.Printf("%s bat to be paid out\n", total.String())
	return &privateRequests, nil
}
