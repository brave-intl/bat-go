package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	geminisettlement "github.com/brave-intl/bat-go/settlement/gemini"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
)

var (
	// GeminiSettlementCmd creates the gemini subcommand
	GeminiSettlementCmd = &cobra.Command{
		Use:   "gemini",
		Short: "provides gemini settlement",
	}

	// UploadGeminiSettlementCmd creates the gemini uphold subcommand
	UploadGeminiSettlementCmd = &cobra.Command{
		Use:   "upload",
		Short: "uploads signed gemini transactions",
		Run:   cmd.Perform("gemini upload", UploadGeminiSettlement),
	}

	// CheckStatusGeminiSettlementCmd creates the gemini checkstatus subcommand
	CheckStatusGeminiSettlementCmd = &cobra.Command{
		Use:   "checkstatus",
		Short: "uploads signed gemini transactions",
		Run:   cmd.Perform("gemini checkstatus", CheckStatusGeminiSettlement),
	}
)

// UploadGeminiSettlement uploads gemini settlement
func UploadGeminiSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	sig, err := cmd.Flags().GetInt("sig")
	if err != nil {
		return err
	}
	allTransactionsFile, err := cmd.Flags().GetString("all-txs-input")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}

	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + "-finished.json"
	}
	return GeminiUploadSettlement(
		cmd.Context(),
		"upload",
		input,
		sig,
		allTransactionsFile,
		out,
	)
}

// CheckStatusGeminiSettlement is the command runner for checking gemini transactions status
func CheckStatusGeminiSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + "-finished.json"
	}
	sig, err := cmd.Flags().GetInt("sig")
	if err != nil {
		return err
	}
	allTxsInput, err := cmd.Flags().GetString("all-txs-input")
	if err != nil {
		return err
	}
	return GeminiUploadSettlement(
		cmd.Context(),
		"checkstatus",
		input,
		sig,
		allTxsInput,
		out,
	)
}

func init() {
	// add complete and transform subcommand
	GeminiSettlementCmd.AddCommand(UploadGeminiSettlementCmd)
	GeminiSettlementCmd.AddCommand(CheckStatusGeminiSettlementCmd)

	// add this command as a settlement subcommand
	SettlementCmd.AddCommand(GeminiSettlementCmd)

	// setup the flags
	uploadBuilder := cmd.NewFlagBuilder(UploadGeminiSettlementCmd)
	statusBuilder := cmd.NewFlagBuilder(CheckStatusGeminiSettlementCmd)
	comboBuilder := uploadBuilder.Concat(statusBuilder)

	comboBuilder.String("input", "",
		"the file or comma delimited list of files that should be utilized").
		Require().
		Env("INPUT")

	comboBuilder.String("out", "./gemini-settlement",
		"the location of the file").
		Env("OUT")

	comboBuilder.String("all-txs-input", "",
		"the original transactions file").
		Require()

	uploadBuilder.Int("sig", 0,
		"signature to choose when uploading transactions (for bulk endpoint usage)")

	comboBuilder.String("out", "./gemini-settlement",
		"the file to output to")
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

	submittedTransactions, submitErr := geminisettlement.IterateRequest(
		ctx,
		action,
		geminiClient,
		signatureSwitch,
		bulkPayoutFiles,
		transactionsMap,
	)
	// write file for upload to eyeshade
	logger.Info().
		Str("files", outPath).
		Msg("outputting files")

	if submittedTransactions != nil {
		for key, txs := range *submittedTransactions {
			outputPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-" + key + ".json"
			err = GeminiWriteTransactions(ctx, outputPath, &txs)
			if err != nil {
				logger.Error().Err(err).Msg("failed to write gemini transactions file")
				return err
			}
		}
	}
	return submitErr
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
