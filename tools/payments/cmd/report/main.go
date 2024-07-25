/*
Report allows payment operators to get the final responses for the submit step

Usage:

report [flags]

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
	"strings"

	"github.com/brave-intl/bat-go/libs/payments"
	paymentscli "github.com/brave-intl/payments-service/tools/payments"
)

func main() {
	ctx := context.Background()

	// command line flags
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

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Environment: %s\n", *env)
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

	status, err := client.GetStatus(ctx, *payoutID)
	if err != nil {
		log.Fatalf("failed to get status: %v\n", err)
	}

	totalTransactions := int(status.SubmitCount)

	// FIXME
	responseStream := payments.SubmitPrefix + *payoutID + payments.ResponseSuffix

	// FIXME default to public key as consumer group?
	err = client.WaitForResponses(ctx, *payoutID, totalTransactions, responseStream, *cg)
	if err != nil {
		log.Fatalf("failed to wait for submit responses: %v\n", err)
	}
}
