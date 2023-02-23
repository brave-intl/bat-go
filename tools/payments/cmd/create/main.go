/*
Create generates a random asymmetric key pair, breaks the private key into operator shares, and outputs
the number of shares at a given threshold to standard out, along with the public key.  After this is
performed the private key is discarded.

Create takes as parameters the threshold and number of operator shares.

Usage:

create [flags]

The flags are:

	-t
		The Shamir share threshold to reconstitute the private key
	-n
		The number of operator shares to output
*/
package main

import (
	"encoding/base64"
	"flag"
	"log"

	"filippo.io/age"
	"github.com/hashicorp/vault/shamir"
)

func main() {
	// command line flags
	threshold := flag.Int(
		"t", 2,
		"the threshold for Shamir shares to reconstitute the private key")
	shares := flag.Int(
		"s", 5,
		"the number of operator shares to generate")
	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Threshold: %d\n", *threshold)
		log.Printf("Shares: %s\n", *shares)
	}

	// generate key
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		log.Fatalf("failed to generate X25519 identity: %s", err.Error())
	}

	// perform Shamir
	operatorShares, err := shamir.Split([]byte(identity.String()), *shares, *threshold)
	if err != nil {
		log.Fatalf("failed to split identity into shamir shares: %s", err.Error())
	}

	for i, v := range operatorShares {
		log.Printf("!!! Operator %d - %s\n", i+1, base64.StdEncoding.EncodeToString(v))
	}
	log.Printf("!!! Public Key - %s\n", identity.Recipient().String())

	if *verbose {
		log.Println("completed create.")
	}
}
