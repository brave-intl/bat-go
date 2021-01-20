package settlement

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	bitflyersettlement "github.com/brave-intl/bat-go/settlement/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
)

var (
	// BitflyerSettlementCmd creates the bitflyer subcommand
	BitflyerSettlementCmd = &cobra.Command{
		Use:   "bitflyer",
		Short: "facilitates bitflyer settlement",
	}

	// UploadBitflyerSettlementCmd creates the bitflyer uphold subcommand
	UploadBitflyerSettlementCmd = &cobra.Command{
		Use:   "upload",
		Short: "uploads signed bitflyer transactions",
		Run:   cmd.Perform("bitflyer upload", UploadBitflyerSettlement),
	}

	// CheckStatusBitflyerSettlementCmd creates the bitflyer checkstatus subcommand
	CheckStatusBitflyerSettlementCmd = &cobra.Command{
		Use:   "checkstatus",
		Short: "uploads signed bitflyer transactions",
		Run:   cmd.Perform("bitflyer checkstatus", CheckStatusBitflyerSettlement),
	}
)

// UploadBitflyerSettlement uploads bitflyer settlement
func UploadBitflyerSettlement(cmd *cobra.Command, args []string) error {
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
	return BitflyerUploadSettlement(
		cmd.Context(),
		"upload",
		input,
		out,
	)
}

// CheckStatusBitflyerSettlement is the command runner for checking bitflyer transactions status
func CheckStatusBitflyerSettlement(cmd *cobra.Command, args []string) error {
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
	return BitflyerUploadSettlement(
		cmd.Context(),
		"checkstatus",
		input,
		out,
	)
}

func init() {
	// add complete and transform subcommand
	BitflyerSettlementCmd.AddCommand(UploadBitflyerSettlementCmd)
	BitflyerSettlementCmd.AddCommand(CheckStatusBitflyerSettlementCmd)

	// add this command as a settlement subcommand
	SettlementCmd.AddCommand(BitflyerSettlementCmd)

	// setup the flags
	uploadCheckStatusBuilder := cmd.NewFlagBuilder(UploadBitflyerSettlementCmd).
		AddCommand(CheckStatusBitflyerSettlementCmd)

	uploadCheckStatusBuilder.Flag().String("input", "",
		"the file or comma delimited list of files that should be utilized. both referrals and contributions should be done in one command in order to group the transactions appropriately").
		Require().
		Bind("input").
		Env("INPUT")

	uploadCheckStatusBuilder.Flag().String("out", "./bitflyer-settlement",
		"the location of the file").
		Bind("out").
		Env("OUT")

	// bitflyer-client-token
	uploadCheckStatusBuilder.Flag().String("bitflyer-client-token", "",
		"bitflyer-client-token holds the uphold token that we want to use to auth the bulk transactions").
		Bind("bitflyer-client-token").
		Env("BITFLYER_CLIENT_TOKEN")
}

// BitflyerUploadSettlement marks the settlement file as complete
func BitflyerUploadSettlement(ctx context.Context, action string, inPath string, outPath string) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	if outPath == "./bitflyer-settlement" {
		// use a file with extension if none is passed
		outPath = "./bitflyer-settlement-complete.json"
	}

	bulkPayoutFiles := strings.Split(inPath, ",")
	bitflyerClient, err := bitflyer.New()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create new bitflyer client")
		return err
	}

	submittedTransactions, submitErr := bitflyersettlement.IterateRequest(
		ctx,
		action,
		bitflyerClient,
		bulkPayoutFiles,
	)
	// write file for upload to eyeshade
	logger.Info().
		Str("files", outPath).
		Msg("outputting files")

	if submittedTransactions != nil {
		for key, txs := range *submittedTransactions {
			if len(txs) > 0 {
				outputPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-" + key + ".json"
				err = BitflyerWriteTransactions(ctx, outputPath, &txs)
				if err != nil {
					logger.Error().Err(err).Msg("failed to write bitflyer transactions file")
					return err
				}
			}
		}
	}
	return submitErr
}

// BitflyerWriteTransactions writes settlement transactions to a json file
func BitflyerWriteTransactions(ctx context.Context, outPath string, metadata *[]settlement.Transaction) error {
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
