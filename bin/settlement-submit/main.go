package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/formatters"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	log "github.com/sirupsen/logrus"
)

var (
	logFile    string
	outputFile string

	verbose             = flag.Bool("v", false, "verbose output")
	inputFile           = flag.String("in", "./contributions-signed.json", "input file path")
	allTransactionsFile = flag.String("alltransactions", "contributions.json", "the file that generated the signatures in the first place")
	provider            = flag.String("provider", "", "the provider that the transactions should be sent to")
	signatureSwitch     = flag.Int("sig", 0, "the signature and corresponding nonce that should be used")
)

func main() {
	log.SetFormatter(&formatters.CliFormatter{})

	flag.Usage = func() {
		log.Printf("Submit signed settlements to " + *provider + ".\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}
	logFile = strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + "-log.json"
	outputFile = strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)) + "-finished.json"

	var err error
	switch *provider {
	case "uphold":
		err = upholdSubmit()
	case "gemini":
		err = cmd.GeminiUploadSettlement(*inputFile, *signatureSwitch, *allTransactionsFile, outputFile)
		<-time.After(time.Second)
	}
	if err != nil {
		log.Fatalln(err)
	}
}

func upholdSubmit() error {
	settlementJSON, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatalln(err)
	}

	var settlementState settlement.State
	err = json.Unmarshal(settlementJSON, &settlementState)
	if err != nil {
		log.Fatalln(err)
	}

	err = settlement.CheckForDuplicates(settlementState.Transactions)
	if err != nil {
		log.Fatalln(err)
	}

	settlementWallet, err := uphold.FromWalletInfo(context.Background(), settlementState.WalletInfo)
	if err != nil {
		log.Fatalln(err)
	}

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		log.Fatalln(err)
	}

	// Read from the transaction log
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var tmp settlement.Transaction
		err = json.Unmarshal(scanner.Bytes(), &tmp)
		if err != nil {
			log.Fatalln(err)
		}
		for i := 0; i < len(settlementState.Transactions); i++ {
			// Only one transaction per channel is allowed per settlement
			if settlementState.Transactions[i].Channel == tmp.Channel {
				settlementState.Transactions[i] = tmp
			}
		}
	}

	allComplete := true
	for i := 0; i < len(settlementState.Transactions); i++ {
		settlementTransaction := &settlementState.Transactions[i]

		err = settlement.SubmitPreparedTransaction(settlementWallet, settlementTransaction)
		if err != nil {
			if errorutils.IsErrInvalidDestination(err) {
				log.Println(err)
				continue
			}
			log.Fatalln(err)
		}

		var out []byte
		out, err = json.Marshal(settlementTransaction)
		if err != nil {
			log.Fatalln(err)
		}

		// Append progress to the log
		_, err = f.Write(append(out, '\n'))
		if err != nil {
			log.Fatalln(err)
		}
		err = f.Sync()
		if err != nil {
			log.Fatalln(err)
		}

		err = settlement.ConfirmPreparedTransaction(settlementWallet, settlementTransaction)
		if err != nil {
			log.Fatalln(err)
		}

		out, err = json.Marshal(settlementTransaction)
		if err != nil {
			log.Fatalln(err)
		}

		// Append progress to the log
		_, err = f.Write(append(out, '\n'))
		if err != nil {
			log.Fatalln(err)
		}
		err = f.Sync()
		if err != nil {
			log.Fatalln(err)
		}

		if !settlementTransaction.IsComplete() {
			allComplete = false
		}
	}

	if allComplete {
		fmt.Println("\nall transactions successfully completed, writing out settlement file")
	} else {
		log.Fatalln("\nnot all transactions successfully completed, rerun to attempt resubmit")
	}

	for i := 0; i < len(settlementState.Transactions); i++ {
		// Redact signed transactions
		settlementState.Transactions[i].SignedTx = ""
	}

	// Write out transactions ready to be submitted to eyeshade
	out, err := json.MarshalIndent(settlementState.Transactions, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}

	err = ioutil.WriteFile(outputFile, out, 0600)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("done!")
	return nil
}
