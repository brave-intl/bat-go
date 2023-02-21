/*
Authorize allows payment operators to add their seal of approval to a list of outgoing transactions.

Authorize will take attested report from stdin and it is expecting valid JSON serialized transactions.

Usage:

authorize [flags]

The flags are:

	-k
		Location on file system of the operators private ED25519 signing key in PEM format.
	-e
		The environment to which the operator is sending approval for transactions.
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
	key := flag.String(
		"k", "test/private.pem",
		"the operator's key file location (ed25519 private key) in PEM format")
	env := flag.String(
		"e", "https://payments.bsg.brave.software",
		"the environment to which the tool will interact")
	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Operator Key File Location: %s\n", *key)
		log.Printf("Environment: %s\n", *env)
	}

	report := payments.AttestedReport{}
	if err := payments.ReadReport(&report, os.Stdin); err != nil {
		log.Fatalf("failed to read report from stdin: %w\n", err)
	}

	if *verbose {
		log.Printf("report stats: %d transactions; %s total bat\n",
			len(report), payments.SumBAT(report...))
	}

	client, err := payments.NewSettlementClient(ctx, *env)
	if err != nil {
		log.Fatalf("failed to create settlement client: %w\n", err)
	}

	priv, err := payments.GetOperatorPrivateKey(*key)
	if err != nil {
		log.Fatalf("failed to parse operator key file: %w\n", err)
	}

	if err := report.Submit(ctx, priv, client); err != nil {
		log.Fatalf("failed to submit report: %w\n", err)
	}

	if *verbose {
		log.Println("completed report submission")
	}
}
