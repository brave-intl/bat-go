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
		The redis cluster addresses comma seperated
	-rp
		The redis cluster password
	-ru
		The redis cluster user
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

	"github.com/brave-intl/bat-go/tools/payments"
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

	redisAddrs := flag.String(
		"ra", "",
		"redis cluster addresses")

	redisPass := flag.String(
		"rp", "",
		"redis cluster password")

	redisUser := flag.String(
		"ru", "",
		"redis cluster username")

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
	client, err := payments.NewSettlementClient(*env, map[string]string{
		"addrs": *redisAddrs, "pass": *redisPass, "username": *redisUser, // client specific configurations
	})
	if err != nil {
		log.Fatalf("failed to create settlement client: %v\n", err)
	}

	if payoutID == nil || strings.TrimSpace(*payoutID) == "" {
		log.Fatal("failed payout id cannot be nil or empty\n")
	}

	wc := &payments.WorkerConfig{
		PayoutID:      *payoutID,
		ConsumerGroup: payments.PrepareStream + "-cg",
		Stream:        payments.PrepareStream,
		Count:         0,
	}

	for _, name := range files {
		func() {
			f, err := os.Open(name)
			if err != nil {
				log.Fatalf("failed to open report file: %v\n", err)
			}
			defer f.Close()

			report := payments.PreparedReport{}
			if err := payments.ReadReport(&report, f); err != nil {
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
		log.Println("completed report preparation")
	}
}
