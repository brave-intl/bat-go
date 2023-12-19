/*
Status allows payment operators to check the status of a payout report in the payments system.

Usage:

status [flags] filename...

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
	-p
		The payout report id to check
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	paymentscli "github.com/brave-intl/bat-go/tools/payments"
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

	payoutID := flag.String(
		"p", "",
		"payout id")

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Environment: %s\n", *env)
	}

	// setup the settlement redis client
	ctx, client, err := paymentscli.NewSettlementClient(ctx, *env, map[string]string{
		"addr": *redisAddr, "pass": *redisPass, "username": *redisUser, // client specific configurations
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

	prepareComplete := status.PrepareCount - (status.PrepareLag + status.PreparePending)
	submitComplete := status.SubmitCount - (status.SubmitLag + status.SubmitPending)

	fmt.Printf("prepare: %d of %d complete, %d not yet processed and %d retrying\n", prepareComplete, status.PrepareCount, status.PrepareLag, status.PreparePending)
	fmt.Printf("submit: %d of %d complete, %d not yet processed and %d retrying\n", submitComplete, status.SubmitCount, status.SubmitLag, status.SubmitPending)
}
