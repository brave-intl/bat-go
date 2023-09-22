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
		The environment is specified as the base URI of the payments service running in the
		nitro enclave.  This should include the protocol, and host at the minimum.  Example:
			https://payments.bsg.brave.software
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
	env := flag.String(
		"e", "https://payments.bsg.brave.software",
		"the environment to which the tool will interact")
	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	redisAddr := flag.String(
		"ra", "",
		"redis address")

	redisPass := flag.String(
		"rp", "",
		"redis password")

	redisUser := flag.String(
		"ru", "",
		"redis username")

	payoutID := flag.String(
		"p", "",
		"payout id")

	flag.Parse()

	// get the list of report files for prepare
	files := flag.Args()

	if *verbose {
		// print out the configuration
		log.Printf("Environment: %s\n", *env)
	}

	// setup the settlement redis client
	client, err := paymentscli.NewSettlementClient(*env, map[string]string{
		"addr": *redisAddr, "pass": *redisPass, "username": *redisUser, // client specific configurations
	})
	if err != nil {
		log.Fatalf("failed to create settlement client: %v\n", err)
	}

	if payoutID == nil || strings.TrimSpace(*payoutID) == "" {
		log.Fatal("failed payout id cannot be nil or empty\n")
	}

	wc := &payments.WorkerConfig{
		PayoutID:      *payoutID,
		ConsumerGroup: paymentscli.PrepareStream + "-cg",
		Stream:        paymentscli.PrepareStream,
		Count:         0,
	}

	for _, name := range files {
		func() {
			f, err := os.Open(name)
			if err != nil {
				log.Fatalf("failed to open report file: %v\n", err)
			}
			defer f.Close()

			report := paymentscli.PreparedReport{}
			if err := paymentscli.ReadReport(&report, f); err != nil {
				log.Fatalf("failed to read report from stdin: %v\n", err)
			}

			wc.Count += len(report)

			if err := report.Prepare(ctx, client); err != nil {
				log.Fatalf("failed to read report from stdin: %v\n", err)
			}
		}()
	}

	err = client.ConfigureWorker(ctx, payments.PrepareConfigStream, wc)
	if err != nil {
		log.Fatalf("failed to write to prepare config stream: %v\n", err)
	}

	if *verbose {
		log.Printf("prepare transactions loaded for %+v\n", wc)
		log.Println("prepare command complete")
	}
}
