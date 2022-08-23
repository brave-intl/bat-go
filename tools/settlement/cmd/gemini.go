package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	rootcmd "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/settlement"
	geminisettlement "github.com/brave-intl/bat-go/tools/settlement/gemini"
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
		Run:   rootcmd.Perform("gemini upload", UploadGeminiSettlement),
	}

	// CheckStatusGeminiSettlementCmd creates the gemini checkstatus subcommand
	CheckStatusGeminiSettlementCmd = &cobra.Command{
		Use:   "checkstatus",
		Short: "uploads signed gemini transactions",
		Run:   rootcmd.Perform("gemini checkstatus", CheckStatusGeminiSettlement),
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

	ctx := context.WithValue(cmd.Context(), appctx.GeminiAPISecretCTXKey, os.Getenv("GEMINI_API_SECRET"))
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, os.Getenv("GEMINI_API_KEY"))

	return GeminiUploadSettlement(
		ctx,
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

	ctx := context.WithValue(cmd.Context(), appctx.GeminiAPISecretCTXKey, os.Getenv("GEMINI_API_SECRET"))
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, os.Getenv("GEMINI_API_KEY"))

	return GeminiUploadSettlement(
		ctx,
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
	uploadCheckStatusBuilder := cmdutils.NewFlagBuilder(UploadGeminiSettlementCmd).
		AddCommand(CheckStatusGeminiSettlementCmd)

	uploadCheckStatusBuilder.Flag().String("input", "",
		"the file or comma delimited list of files that should be utilized").
		Require().
		Bind("input").
		Env("INPUT")

	uploadCheckStatusBuilder.Flag().String("out", "./gemini-settlement",
		"the location of the file").
		Bind("out").
		Env("OUT")

	uploadCheckStatusBuilder.Flag().String("all-txs-input", "",
		"the original transactions file").
		Bind("all-txs-input").
		Require()

	uploadCheckStatusBuilder.Flag().Int("sig", 0,
		"signature to choose when uploading transactions (for bulk endpoint usage)").
		Bind("sig")
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
	transactionsMap, err := geminiMapTransactionsToID(settlementTransactions)
	if err != nil {
		logger.Error().Err(err).Msg("failed validate and convert transactions")
		return err
	}

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

	err = WriteCategorizedTransactions(ctx, outPath, submittedTransactions)
	if err != nil {
		return err
	}
	return submitErr
}

// geminiMapTransactionsToID creates a map of guid's to transactions
func geminiMapTransactionsToID(transactions []settlement.AntifraudTransaction) (map[string]custodian.Transaction, error) {
	transactionsMap := make(map[string]custodian.Transaction)
	for _, atx := range transactions {
		tx, err := atx.ToTransaction()
		if err != nil {
			return transactionsMap, err
		}
		transactionsMap[gemini.GenerateTxRef(&tx)] = tx
	}
	return transactionsMap, nil
}
