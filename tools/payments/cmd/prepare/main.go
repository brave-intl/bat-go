/*
Prepare allows payment operators to upload the prepared payout report to the payments system.

Prepare reads the payment report from the file provided on command line and is expecting valid JSON serialized transactions.

Usage:

prepare [flags] filename...

The flags are:

	-v
		verbose logging enabled
	-e
		The environment to which the operator is sending transactions to be put in prepared state.
	-ra
		The redis addresses comma seperated
	-rp
		The redis password
	-ru
		The redis user
*/

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/libs/payments"
	paymentscli "github.com/brave-intl/bat-go/tools/payments"
)

func main() {
	ctx := context.Background()

	// command line flags
	key := flag.String(
		"k", "test/private.pem",
		"the operator's key file location (ed25519 private key) in PEM format")

	env := flag.String(
		"e", "local",
		"the environment to which the tool will interact")

	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	redisAddr := flag.String(
		"ra", "127.0.0.1:6380",
		"redis address")

	redisPass := flag.String(
		"rp", "",
		"redis password")

	redisUser := flag.String(
		"ru", "",
		"redis username")

	pcr2 := flag.String(
		"pcr2", "", "the hex PCR2 value for this enclave")

	payoutID := flag.String(
		"p", "",
		"payout id")

	cg := flag.String(
		"cg", "cli",
		"consumer group suffix")

	resubmit := flag.Bool(
		"resubmit", false,
		"resubmit to prepare stream")

	flag.Parse()

	// get the list of report files for prepare
	files := flag.Args()

	if *verbose {
		// print out the configuration
		log.Printf("Environment: %s\n", *env)
		log.Printf("Operator Key File Location: %s\n", *key)
	}

	// setup the settlement redis client
	ctx, client, err := paymentscli.NewSettlementClient(ctx, *env, map[string]string{
		"addr": *redisAddr, "pass": *redisPass, "username": *redisUser, "pcr2": *pcr2, // client specific configurations
	})
	if err != nil {
		log.Fatalf("failed to create settlement client: %v\n", err)
	}

	if payoutID == nil || strings.TrimSpace(*payoutID) == "" {
		log.Fatal("failed payout id cannot be nil or empty\n")
	}

	firstRun := true
	// FIXME
	responseStream := payments.PreparePrefix + *payoutID + payments.ResponseSuffix
	responseFile := responseStream + ".log"
	if _, err := os.Stat(responseFile); err == nil {
		firstRun = false
		if !*resubmit {
			log.Println("not first run, skipping add to prepare stream")
		}
	}

	totalTransactions := 0
	f, err := os.Open(files[0])
	if err != nil {
		log.Fatalf("failed to open report file: %v\n", err)
	}
	defer f.Close()

	report := paymentscli.PreparedReport{}
	if err := paymentscli.ReadReport(&report, f); err != nil {
		log.Fatalf("failed to read report from stdin: %v\n", err)
	}
	if err := report.Validate(); err != nil {
		log.Fatalf("failed to validate report: %v\n", err)
	}

	if report[0].PayoutID != *payoutID {
		log.Fatalf("payoutID did not match report: %s\n", report[0].PayoutID)
	}

	totalTransactions += len(report)
	if firstRun || *resubmit {
		priv, err := paymentscli.GetOperatorPrivateKey(*key)
		if err != nil {
			log.Fatalf("failed to parse operator key file: %v\n", err)
		}

		if err := report.Prepare(ctx, priv, client); err != nil {
			log.Fatalf("failed to read report from stdin: %v\n", err)
		}

		wc := &payments.WorkerConfig{
			PayoutID:      *payoutID,
			ConsumerGroup: payments.PreparePrefix + *payoutID + "-cg",
			Stream:        payments.PreparePrefix + *payoutID,
			Count:         len(report),
		}

		err = client.ConfigureWorker(ctx, payments.PrepareConfigStream, wc)
		if err != nil {
			log.Fatalf("failed to write to prepare config stream: %v\n", err)
		}
		if *verbose {
			log.Printf("prepare transactions loaded for %+v\n", payoutID)
		}

		os.Create(responseFile)
	}

	// FIXME default to public key as consumer group?
	err = client.WaitForResponses(ctx, *payoutID, totalTransactions, responseStream, *cg)
	if err != nil {
		log.Fatalf("failed to wait for prepare responses: %v\n", err)
	}

	// perform validations

	// read in responseFile as attested report
	attestedReportFile, err := os.Open(responseFile)
	if err != nil {
		log.Fatalf("failed to open attested report file: %v\n", err)
	}
	defer attestedReportFile.Close()

	// parse the attested report
	attestedReport := paymentscli.AttestedReport{}
	if err := paymentscli.ReadReportFromResponses(&attestedReport, attestedReportFile); err != nil {
		log.Fatalf("failed to read attested report: %v\n", err)
	}

	if *verbose {
		log.Printf("attested report stats: %d transactions; %s total bat\n",
			len(attestedReport), attestedReport.SumBAT())
		log.Printf("prepared report stats: %d transactions; %s total bat\n",
			len(report), report.SumBAT())
	}

	// compare performs automated checks to validate reports
	if err := paymentscli.Compare(report, attestedReport); err != nil {
		log.Fatalf("failed to compare reports: %v\n", err)
	}

	if *verbose {
		log.Println("prepare command complete")
	}
}
