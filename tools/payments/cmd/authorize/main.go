/*
Authorize allows payment operators to add their seal of approval to a list of outgoing transactions.

Authorize will take attested report from the command line and it is expecting valid JSON serialized transactions.

Usage:

authorize [flags] filename...

The flags are:

	-v
		verbose logging enabled
	-k
		Location on file system of the operators private ED25519 signing key in PEM format.
	-e
		The environment to which the operator is sending approval for transactions.
	-ra
		The redis address
	-rp
		The redis password
	-ru
		The redis user
	-p
		The payout id
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

	redisAddr := flag.String(
		"ra", "127.0.0.1:6380",
		"redis address")

	redisPass := flag.String(
		"rp", "",
		"redis password")

	redisUser := flag.String(
		"ru", "",
		"redis username")

	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	pcr2 := flag.String(
		"pcr2", "", "the hex PCR2 value for this enclave")

	payoutID := flag.String(
		"p", "",
		"payout id")

	flag.Parse()

	// get the list of report files for prepare
	files := flag.Args()

	if *verbose {
		// print out the configuration
		log.Printf("Operator Key File Location: %s\n", *key)
		log.Printf("Redis: %s, %s\n", *redisAddr, *redisUser)
	}

	// setup the settlement redis client
	ctx, client, err := paymentscli.NewSettlementClient(ctx, *env, map[string]string{
		"addr": *redisAddr, "pass": *redisPass, "username": *redisUser, "pcr2": *pcr2 // client specific configurations
	})
	if err != nil {
		log.Fatalf("failed to create settlement client: %v\n", err)
	}

	if payoutID == nil || strings.TrimSpace(*payoutID) == "" {
		log.Fatal("failed payout id cannot be nil or empty\n")
	}

	for _, name := range files {
		func() {
			f, err := os.Open(name)
			if err != nil {
				log.Fatalf("failed to open report file: %v\n", err)
			}
			defer f.Close()

			var report paymentscli.AttestedReport
			if err := paymentscli.ReadReportFromResponses(&report, f); err != nil {
				log.Fatalf("failed to read report from stdin: %v\n", err)
			}

			if report[0].PayoutID != *payoutID {
				log.Fatalf("payoutID did not match report: %s\n", report[0].PayoutID)
			}

			if *verbose {
				log.Printf("report stats: %d transactions; %s total bat\n", len(report), report.SumBAT())
			}

			priv, err := paymentscli.GetOperatorPrivateKey(*key)
			if err != nil {
				log.Fatalf("failed to parse operator key file: %v\n", err)
			}

			if err := report.Submit(ctx, priv, client); err != nil {
				log.Fatalf("failed to submit report: %v\n", err)
			}

			wc := &payments.WorkerConfig{
				PayoutID:      *payoutID,
				ConsumerGroup: payments.SubmitPrefix + *payoutID + "-cg",
				Stream:        payments.SubmitPrefix + *payoutID,
				Count:         len(report),
			}

			err = client.ConfigureWorker(ctx, payments.SubmitConfigStream, wc)
			if err != nil {
				log.Fatalf("failed to write to submit config stream: %v\n", err)
			}

			if *verbose {
				log.Printf("submit transactions loaded for %+v\n", wc)
			}
		}()
	}

	if *verbose {
		log.Println("authorize command complete")
	}
}
