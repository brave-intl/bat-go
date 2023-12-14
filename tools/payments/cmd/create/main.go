/*
Create generates a random asymmetric key pair, breaks the private key into operator shares, and outputs
the number of shares at a given threshold to standard out, along with the public key.  After this is
performed the private key is discarded.

Create takes as parameters the threshold and number of operator shares.

Usage:

create-vault [flags] [pubkeys ...]

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
	"fmt"
	"io"
	"log"
	"os"

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

	publicKeys := flag.Args()

	if *verbose {
		// print out the configuration
		log.Printf("Threshold: %d\n", *threshold)
		log.Printf("Shares: %s\n", *shares)
	}

	if *shares != len(publicKeys) {
		log.Fatalf("invalid number of shares specified for number of public key files")
	}

	// load up the operator recipients (x25519 public keys)
	operatorRecipients := []*age.X25519Recipient{}
	for _, key := range publicKeys {
		recipient, err := age.ParseX25519Recipient(key)
		if err != nil {
			log.Fatalf("Failed to parse public key %q: %v", key, err)
		}
		operatorRecipients = append(operatorRecipients, recipient)
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
		// open output file for this operator
		f, err := os.Create(fmt.Sprintf("share-%s.enc", operatorRecipients[i].String()))
		if err != nil {
			log.Fatalf("failed to open operator receipient share file", err.Error())
		}

		// encrypt each with an operator recipient
		w, err := age.Encrypt(f, operatorRecipients[i])
		if err != nil {
			log.Fatalf("failed to encrypt to receipient share file", err.Error())
		}

		if _, err := io.WriteString(w, base64.StdEncoding.EncodeToString(v)); err != nil {
			log.Fatalf("failed to write ciphertext to receipient share file", err.Error())
		}

		if err := w.Close(); err != nil {
			log.Fatalf("failed to close receipient share file", err.Error())
		}

	}
	log.Printf("!!! Public Key - %s\n", identity.Recipient().String())

	if *verbose {
		log.Println("completed create.")
	}
}
