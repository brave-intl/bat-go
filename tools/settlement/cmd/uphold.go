package settlement

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	rootcmd "github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/tools/settlement"
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
		Run:   rootcmd.Perform("upload", RunUpholdUpload),
	}
)

func init() {
	UpholdCmd.AddCommand(
		UpholdUploadCmd,
	)

	SettlementCmd.AddCommand(
		UpholdCmd,
	)

	uploadBuilder := cmdutils.NewFlagBuilder(UpholdUploadCmd)

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
	progChan := logging.UpholdReportProgress(ctx, progressDuration)
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

func recordProgress(f *os.File, settlementTransaction *custodian.Transaction) error {
	var out []byte
	out, err := json.Marshal(settlementTransaction)
	if err != nil {
		return fmt.Errorf("failed to unmarshal settlement transaction: %v", err)
	}

	// Append progress to the log
	_, err = f.Write(append(out, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to output log: %v", err)
	}
	err = f.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync output log to disk: %v", err)
	}
	return nil
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
	logger.Info().Msg("beginning uphold upload")

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
	logger.Info().Msg("scanning stateful logs to establish transaction status")
	scanner := bufio.NewScanner(f)
	isResubmit := false
	for scanner.Scan() {
		var tmp custodian.Transaction
		err = json.Unmarshal(scanner.Bytes(), &tmp)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to scan the transaction log")
		}
		for i := 0; i < len(settlementState.Transactions); i++ {
			// Only one transaction per channel is allowed per settlement
			if settlementState.Transactions[i].Channel == tmp.Channel {
				isResubmit = true
				settlementState.Transactions[i] = tmp
			}
		}
	}

	// Optimize the case where we are rerunning by creating a truncated snapshot of the last state
	if isResubmit {
		backupFile := strings.TrimSuffix(logFile, filepath.Ext(logFile)) + "-backup.json"
		backupF, err := os.OpenFile(backupFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to open backup file")
		}

		// Reset log offset to 0, append to the backup we just opened
		_, err = f.Seek(0, 0)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to seek log back to start")
		}
		nBytes, err := io.Copy(backupF, f)
		if nBytes <= 0 || err != nil {
			logger.Panic().Err(err).Msg("failed to backup log")
		}
		err = backupF.Sync()
		if err != nil {
			logger.Panic().Err(err).Msg("failed to sync backup log to disk")
		}

		tmpFile := strings.TrimSuffix(logFile, filepath.Ext(logFile)) + "-tmp.json"
		tmpF, err := os.OpenFile(tmpFile, os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to create temporary file")
		}

		// Snapshot the fully caught up state into a temporary file
		for i := 0; i < len(settlementState.Transactions); i++ {
			err = recordProgress(tmpF, &settlementState.Transactions[i])
			if err != nil {
				logger.Panic().Err(err).Msg("failed to snapshot state")
			}
		}

		// Replace our log file handle with the temporary file handle
		f = tmpF

		// Rename the temporary file, replacing the original log with the truncated snapshot
		// NOTE: this is only done after we've successfully written to the new temporary file to ensure
		// that even in pathological cases we always have a valid log file to resume from
		err = os.Rename(tmpFile, logFile)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to replace the log")
		}
	}

	var total = len(settlementState.Transactions)

	// Attempt to move all transactions into a processing state
	allFinalized := true
	someProcessing := false
	progress := logging.UpholdProgressSet{
		Progress: []logging.UpholdProgress{{
			Message: "Successes",
			Count:   0,
		}},
	}
	for i := 0; i < total; i++ {
		settlementTransaction := &settlementState.Transactions[i]

		if settlementTransaction.IsComplete() || settlementTransaction.IsFailed() {
			continue
		}

		err = settlement.SubmitPreparedTransaction(ctx, settlementWallet, settlementTransaction)
		if err != nil {
			logger.Error().Err(err).Msg("unanticipated error")
			settlementTransaction.FailureReason = fmt.Sprintf("unanticipated error: %e", err)
			allFinalized = false
			continue
		}

		err = recordProgress(f, settlementTransaction)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to record progress")
		}

		err = settlement.ConfirmPreparedTransaction(ctx, settlementWallet, settlementTransaction, isResubmit)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to confirm prepared transaction")
		}

		err = recordProgress(f, settlementTransaction)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to record progress")
		}

		// We will later attempt to resolve all processing to complete or failed
		if settlementTransaction.IsProcessing() {
			someProcessing = true
		} else if !settlementTransaction.IsComplete() && !settlementTransaction.IsFailed() {
			allFinalized = false
		}

		// Progress is tracked on an error by error basis based on a string
		// comparison of errors. Each iteration we need to see if the error
		// message we received matches any messages we have already received. If
		// yes, increment the count. If no, add this new error with count 1. If
		// there was no error, increment or create a success progress entry.
		for p := 0; p < len(progress.Progress); p++ {
			existingProgressEntry := progress.Progress[p]
			progressMessage := settlementTransaction.FailureReason
			if settlementTransaction.FailureReason == "" {
				progressMessage = "Successes"
			}

			if existingProgressEntry.Message == progressMessage {
				progress.Progress[p].Count++
				break
			} else if p == len(progress.Progress)-1 {
				progress.Progress = append(progress.Progress, logging.UpholdProgress{
					Message: progressMessage,
					Count:   1,
				})
			}
		}

		// perform progress logging
		logging.UpholdSubmitProgress(ctx, progress)
	}

	// While there are transactions in the processing state, attempt to resolve them to complete or failed
	for someProcessing {
		someProcessing = false
		for i := 0; i < total; i++ {
			settlementTransaction := &settlementState.Transactions[i]

			if settlementTransaction.IsProcessing() {
				logger.Info().Msg("reattempting to confirm transaction in progress")
				// Confirm will first check if the transaction has already been confirmed
				err = settlement.ConfirmPreparedTransaction(ctx, settlementWallet, settlementTransaction, true)
				if err != nil {
					logger.Panic().Err(err).Msg("failed to confirm prepared transaction")
				}

				err = recordProgress(f, settlementTransaction)
				if err != nil {
					logger.Panic().Err(err).Msg("failed to record progress")
				}

				if settlementTransaction.IsProcessing() {
					someProcessing = true
				}
			}
		}
	}

	if allFinalized {
		logger.Info().Msg("all transactions finalized, writing out settlement file")
	} else {
		logger.Error().Msg("not all transactions are finalized, rerun to resubmit")
		return nil
	}

	transactionsMap := make(map[string][]custodian.Transaction)
	for i := 0; i < len(settlementState.Transactions); i++ {
		logger.Info().Msg("redacting transactions in log files")
		// Redact signed transactions
		settlementState.Transactions[i].SignedTx = ""

		// Group by status
		logger.Info().Msg("grouping transactions by status")
		status := settlementState.Transactions[i].Status
		transactionsMap[status] = append(transactionsMap[status], settlementState.Transactions[i])
	}

	for key, txs := range transactionsMap {
		outputFile := outputFilePrefix + "-" + key + ".json"
		logger.Info().Msg(fmt.Sprintf("writing out transactions to %s for eyeshade", outputFile))

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
