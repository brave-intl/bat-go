/*
Configure encrypts a configuration file for consumption by the payments service, the output of which
is then uploaded to s3 and consumed by the payments service.

Create takes as parameters the public key output from the create command, and a configuration file.

Usage:

create [flags] [args]

The flags are:

	-k
		The public key of the payments service (output from create command)

The arguments are configuration files which are to be encrypted.
*/
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"filippo.io/age"
)

func main() {
	// command line flags
	publicKey := flag.String(
		"k", "",
		"the public key of the payment service (from create command)")
	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	files := flag.Args()

	if *verbose {
		// print out the configuration
		log.Printf("Public Key: %s\n", *publicKey)
		log.Printf("Configuration Files: %s\n", files)
	}

	recipient, err := age.ParseX25519Recipient(*publicKey)
	if err != nil {
		log.Fatalf("Failed to parse public key %q: %v", publicKey, err)
	}

	for _, f := range files {
		// open configuration file
		in, err := os.Open(f)
		if err != nil {
			log.Fatalf("Failed to open configuration: %s", err)
		}

		// open output file
		out, err := os.Create(f + ".enc")
		if err != nil {
			log.Fatalf("Failed to open output file: %s", err)
		}

		w, err := age.Encrypt(out, recipient)
		if err != nil {
			log.Fatalf("Failed to create encrypted file: %v", err)
		}

		if _, err := io.Copy(w, in); err != nil {
			log.Fatalf("Failed to write encrypted file: %v")
		}

		// close encrypted file
		if err := w.Close(); err != nil {
			log.Fatalf("Failed to close encrypted file: %v", err)
		}

		// close configuration file
		if err := in.Close(); err != nil {
			log.Fatalf("Failed to close file: %v", err)
		}

		fmt.Printf("Encrypted %s: %s\n", f, f+".enc")
	}

	if *verbose {
		log.Println("completed configure.")
	}
}
