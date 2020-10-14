package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	settlementcmd "github.com/brave-intl/bat-go/cmd/settlement"
	"github.com/brave-intl/bat-go/settlement"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
)

var (
	logFile    string
	outputFile string

	verbose             = flag.Bool("v", false, "verbose output")
	progressDuration    = flag.Duration("p", time.Duration(0), "duration for progress logging")
	inputFile           = flag.String("in", "./contributions-signed.json", "input file path")
	allTransactionsFile = flag.String("alltransactions", "contributions.json", "the file that generated the signatures in the first place")
	provider            = flag.String("provider", "", "the provider that the transactions should be sent to")
	signatureSwitch     = flag.Int("sig", 0, "the signature and corresponding nonce that should be used")
)

func main() {

	flag.Usage = func() {
		log.Printf("Submit signed settlements to " + *provider + ".\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	// setup context for logging, debug and progress
	ctx := context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, verbose)

	// setup progress logging
	progChan := logging.ReportProgress(ctx, *progressDuration)
	ctx = context.WithValue(ctx, appctx.ProgressLoggingCTXKey, progChan)

	// setup logger, with the context that has the logger
	ctx, logger := logging.SetupLogger(ctx)

	logFile = strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + "-log.json"
	outputFile = strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + "-finished.json"

	var err error
	switch *provider {
	case "uphold":
		err = upholdSubmit(ctx)
	case "gemini":
		ctx := context.Background()
		err = settlementcmd.GeminiUploadSettlement(ctx, *inputFile, *signatureSwitch, *allTransactionsFile, outputFile)
	}
	if err != nil {
		logger.Panic().Err(err).Msg("error encountered running settlement-submit")
	}
}

func upholdSubmit(ctx context.Context) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	settlementJSON, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to read input file")
	}

	var settlementState settlement.State
	err = json.Unmarshal(settlementJSON, &settlementState)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to unmarshal input file")
	}

	err = settlement.CheckForDuplicates(settlementState.Transactions)
	if err != nil {
		logger.Panic().Err(err).Msg("failed duplicate transaction check")
	}

	settlementWallet, err := uphold.FromWalletInfo(context.Background(), settlementState.WalletInfo)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to make settlement wallet")
	}

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to create output file")
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

	allComplete := true
	for i := 0; i < len(settlementState.Transactions); i++ {
		settlementTransaction := &settlementState.Transactions[i]

		err = settlement.SubmitPreparedTransaction(settlementWallet, settlementTransaction)
		if err != nil {
			if errorutils.IsErrInvalidDestination(err) {
				logger.Info().Err(err).Msg("invalid destination, skipping")
				continue
			}
			logger.Panic().Err(err).Msg("unanticipated error")
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

		err = settlement.ConfirmPreparedTransaction(settlementWallet, settlementTransaction)
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

		if !settlementTransaction.IsComplete() {
			allComplete = false
		}

		// perform progress logging
		logging.SubmitProgress(ctx, i, total)
	}

	if allComplete {
		logger.Info().Msg("all transactions successfully completed, writing out settlement file")
	} else {
		logger.Panic().Msg("not all transactions are successfully completed, rerun to resubmit")
	}

	for i := 0; i < len(settlementState.Transactions); i++ {
		// Redact signed transactions
		settlementState.Transactions[i].SignedTx = ""
	}

	// Write out transactions ready to be submitted to eyeshade
	out, err := json.MarshalIndent(settlementState.Transactions, "", "    ")
	if err != nil {
		logger.Panic().Err(err).Msg("failed to marshal settlement transactions to eyeshade input")
	}

	err = ioutil.WriteFile(outputFile, out, 0600)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to write out settlement transactions to eyeshade input")
	}

	logger.Info().Msg("done!")
	return nil
}
