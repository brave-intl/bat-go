/*
Prepare allows payment operators to upload the prepared payout report to the payments system.

Prepare reads the payment report through os.Stdin and is expecting valid JSON serialized transactions.

Usage:

prepare [flags]

The flags are:

	-e
		The environment to which the operator is sending transactions to be put in prepared state.
		The environment is specified as the base URI of the payments service running in the
		nitro enclave.  This should include the protocol, and host at the minimum.  Example:
			https://payments.bsg.brave.software

*/

package main

import (
	"context"
	"flag"
	"log"
	"os"

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

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Environment: %s\n", *env)
	}

	report := payments.PreparedReport{}
	if err := payments.ReadReport(&report, os.Stdin); err != nil {
		log.Fatalf("failed to read report from stdin: %w\n", err)
	}

	client, err := payments.NewSettlementClient(ctx, *env)
	if err != nil {
		log.Fatalf("failed to create settlement client: %w\n", err)
	}

	if err := report.Prepare(ctx, client); err != nil {
		log.Fatalf("failed to read report from stdin: %w\n", err)
	}

	if *verbose {
		log.Println("completed report preparation")
	}
}
