package settlement

import (
	"bufio"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/spf13/cobra"
)

var (
	// UpholdCmd uphold subcommand
	UpholdCmd = &cobra.Command{
		Use:   "uphold",
		Short: "uphold sub command",
	}
	// UpholdUploadCmd uphold upload subcommand
	UpholdUploadCmd = &cobra.Command{
		Use:   "upload",
		Short: "upload to uphold",
		Run:   cmd.Perform("upload", RunUpholdUpload),
	}
)

func init() {
	UpholdCmd.AddCommand(
		UpholdUploadCmd,
	)

	SettlementCmd.AddCommand(
		UpholdCmd,
	)

	uploadBuilder := cmd.NewFlagBuilder(UpholdUploadCmd)

	uploadBuilder.Flag().Bool("verbose", false,
		"how verbose logging should be").
		Bind("verbose")

	uploadBuilder.Flag().String("input", "",
		"input file to submit to a given provider").
		Bind("input").
		Require()

	uploadBuilder.Flag().String("progress", "1s",
		"how often progress should be printed out").
		Bind("progress")
}

// RunUpholdUpload the runner that the uphold upload command calls
func RunUpholdUpload(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return err
	}
	progress, err := cmd.Flags().GetString("progress")
	if err != nil {
		return err
	}
	inputFile, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	// setup context for logging, debug and progress
	ctx = context.WithValue(ctx, appctx.DebugLoggingCTXKey, verbose)

	// setup progress logging
	progressDuration, err := time.ParseDuration(progress)
	if err != nil {
		return err
	}
	progChan := logging.ReportProgress(ctx, progressDuration)
	ctx = context.WithValue(ctx, appctx.ProgressLoggingCTXKey, progChan)

	logFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + "-log.json"
	outputFilePrefix := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))

	return UpholdUpload(
		ctx,
		inputFile,
		logFile,
		outputFilePrefix,
	)
}

// UpholdUpload uploads transactions to uphold
func UpholdUpload(
	ctx context.Context,
	inputFile string,
	logFile string,
	outputFilePrefix string,
) error {

	// setup logger, with the context that has the logger
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	settlementJSON, err := ioutil.ReadFile(inputFile)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to read input file")
	}

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to create output file")
	}

	var settlementState settlement.State
	err = json.Unmarshal(settlementJSON, &settlementState)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to unmarshal input file")
	}

	settlementWallet, err := uphold.FromWalletInfo(ctx, settlementState.WalletInfo)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to make settlement wallet")
	}

	// Read from the transaction log
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var tmp settlement.Transaction
		err = json.Unmarshal(scanner.Bytes(), &tmp)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to scan the transaction log")
		}
		for i := 0; i < len(settlementState.Transactions); i++ {
			// Only one transaction per channel is allowed per settlement
			if settlementState.Transactions[i].Channel == tmp.Channel {
				settlementState.Transactions[i] = tmp
			}
		}
	}

	var total = len(settlementState.Transactions)

	allFinalized := true
	for i := 0; i < total; i++ {
		settlementTransaction := &settlementState.Transactions[i]

		if settlementTransaction.IsComplete() || settlementTransaction.IsFailed() {
			continue
		}

		err = settlement.SubmitPreparedTransaction(ctx, settlementWallet, settlementTransaction)
		if err != nil {
			logger.Error().Err(err).Msg("unanticipated error")
			allFinalized = false
			continue
		}

		var out []byte
		out, err = json.Marshal(settlementTransaction)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to unmarshal settlement transaction")
		}

		// Append progress to the log
		_, err = f.Write(append(out, '\n'))
		if err != nil {
			logger.Panic().Err(err).Msg("failed to write to output log")
		}
		err = f.Sync()
		if err != nil {
			logger.Panic().Err(err).Msg("failed to sync output log to disk")
		}

		err = settlement.ConfirmPreparedTransaction(ctx, settlementWallet, settlementTransaction)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to confirm prepared transaction")
		}

		out, err = json.Marshal(settlementTransaction)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to marshal prepared transaction")
		}

		// Append progress to the log
		_, err = f.Write(append(out, '\n'))
		if err != nil {
			logger.Panic().Err(err).Msg("failed to write progress to output log")
		}
		err = f.Sync()
		if err != nil {
			logger.Panic().Err(err).Msg("failed to sync output log")
		}

		if !settlementTransaction.IsComplete() && !settlementTransaction.IsFailed() {
			allFinalized = false
		}

		// perform progress logging
		logging.SubmitProgress(ctx, i, total)
	}

	if allFinalized {
		logger.Info().Msg("all transactions finalized, writing out settlement file")
	} else {
		logger.Error().Msg("not all transactions are finalized, rerun to resubmit")
		return nil
	}

	transactionsMap := make(map[string][]settlement.Transaction)
	for i := 0; i < len(settlementState.Transactions); i++ {
		// Redact signed transactions
		settlementState.Transactions[i].SignedTx = ""

		// Group by status
		status := settlementState.Transactions[i].Status
		transactionsMap[status] = append(transactionsMap[status], settlementState.Transactions[i])
	}

	for key, txs := range transactionsMap {
		outputFile := outputFilePrefix + "-" + key + ".json"

		// Write out transactions ready to be submitted to eyeshade
		out, err := json.MarshalIndent(txs, "", "    ")
		if err != nil {
			logger.Panic().Err(err).Msg("failed to marshal settlement transactions to eyeshade input")
		}

		err = ioutil.WriteFile(outputFile, out, 0600)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to write out settlement transactions to eyeshade input")
		}
	}

	logger.Info().Msg("done!")
	return nil
}
