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
		The environment is specified as the base URI of the payments service running in the
		nitro enclave.  This should include the protocol, and host at the minimum.  Example:
			https://payments.bsg.brave.software
	-ra
		The redis cluster addresses comma seperated
	-rp
		The redis cluster password
	-ru
		The redis cluster user
*/
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/brave-intl/bat-go/tools/payments"
	uuid "github.com/satori/go.uuid"
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

	redisAddrs := flag.String(
		"ra", "",
		"redis cluster addresses")

	redisPass := flag.String(
		"rp", "",
		"redis cluster password")

	redisUser := flag.String(
		"ru", "",
		"redis cluster username")

	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	// get the list of report files for prepare
	files := flag.Args()

	if *verbose {
		// print out the configuration
		log.Printf("Operator Key File Location: %s\n", *key)
		log.Printf("Redis: %s, %s\n", *redisAddrs, *redisUser)
	}

	// setup the settlement redis client
	client, err := payments.NewSettlementClient(*env, map[string]string{
		"addrs": *redisAddrs, "pass": *redisPass, "username": *redisUser, // client specific configurations
	})
	if err != nil {
		log.Fatalf("failed to create settlement client: %v\n", err)
	}

	wc := &payments.WorkerConfig{
		PayoutID:      uuid.NewV4().String(),
		ConsumerGroup: payments.SubmitStream + "-cg",
		Stream:        payments.SubmitStream,
		Count:         0,
	}

	for _, name := range files {
		func() {
			f, err := os.Open(name)
			if err != nil {
				log.Fatalf("failed to open report file: %v\n", err)
			}
			defer f.Close()

			var report payments.AttestedReport
			if err := payments.ReadReport(&report, f); err != nil {
				log.Fatalf("failed to read report from stdin: %v\n", err)
			}

			wc.Count += len(report)

			if *verbose {
				log.Printf("report stats: %d transactions; %s total bat\n", len(report), report.SumBAT())
			}

			priv, err := payments.GetOperatorPrivateKey(*key)
			if err != nil {
				log.Fatalf("failed to parse operator key file: %v\n", err)
			}

			if err := report.Submit(ctx, priv, client); err != nil {
				log.Fatalf("failed to submit report: %v\n", err)
			}
		}()
	}

	err = client.ConfigureWorker(ctx, payments.SubmitConfigStream, wc)
	if err != nil {
		log.Fatalf("failed to write to submit config stream: %v\n", err)
	}

	if *verbose {
		log.Printf("submit transactions loaded for %+v\n", wc)
		log.Println("completed report submission")
	}
}
