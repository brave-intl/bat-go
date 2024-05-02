/*
Vault instructs the enclave to generate and approve a random asymmetric key
pair, break the private key into operator shares, return the shares after they
have been encrypted with provided operator keys, and then discard the private
key so that the operators are needed to access it again. It must be run by
multiple operators with the same arguments to take effect.

# Create takes as parameters a set of operator pubkeys and a threshold

Usage:

create-vault [flags] create [pubkeyFile]
create-vault [flags] approve [pubkeyFile]

The flags are:

	-t
		The Shamir share threshold to reconstitute the private key
	-k
		Location on file system of the operators private ED25519 signing key in PEM format.
	-p
		The public key for the vault private key that is being approved. Only needed for approve subcommand.
*/
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	client "github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/payments"
	paymentscli "github.com/brave-intl/bat-go/tools/payments"
)

func main() {
	ctx := context.Background()

	// command line flags
	operatorKey := flag.String(
		"k", "test/private.pem",
		"the operator's key file location (ed25519 private key) in PEM format")
	threshold := flag.Int(
		"t", 2,
		"the threshold for Shamir shares to reconstitute the private key")
	publicKey := flag.String(
		"p", "",
		"the public key for the vault private key that is being approved. only needed for approve subcommand")
	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		log.Fatal("Expected subcommand and key file arguments")
	}

	f, err := os.Open(args[1])
	if err != nil {
		log.Fatalf("failed to open key file: %v\n", err)
	}
	defer f.Close()

	keys := paymentscli.OperatorKeys{}
	if err := json.NewDecoder(f).Decode(keys); err != nil {
		log.Fatalf("failed to parse operator key file: %w", err)
	}

	if *verbose {
		// print out the configuration
		log.Printf("Threshold: %d\n", *threshold)
		log.Printf("Shares: %s\n", len(keys.Keys))
	}
	if len(keys.Keys) < 2 {
		log.Fatalf("insufficient number of keys to create a share")
	}

	var resp *http.Response

	switch args[0] {
	case "create":
		resp = doRequestWithSignature(
			ctx,
			*operatorKey,
			"/v1/vault/create",
			payments.CreateVaultRequest{
				Operators: keys.Keys,
				Threshold: *threshold,
			},
		)
	case "approve":
		if len(*publicKey) == 0 {
			log.Fatal("public key flag must be defined with -p")
		}
		resp = doRequestWithSignature(
			ctx,
			*operatorKey,
			"/v1/vault/approve",
			payments.ApproveVaultRequest{
				Operators: keys.Keys,
				Threshold: *threshold,
				PublicKey: *publicKey,
			},
		)
	default:
		log.Fatal("unrecognized subcommand. options are create and approve")
	}

	log.Printf("%+v", resp)

	if *verbose {
		log.Println("completed create.")
	}
}

func doRequestWithSignature(ctx context.Context, key, path string, data interface{}) *http.Response {
	priv, err := paymentscli.GetOperatorPrivateKey(key)
	if err != nil {
		log.Fatalf("failed to parse operator key file: %v\n", err)
	}
	var (
		// dateHeader needs to be lowercase to pass the signing verifier validation.
		dateHeader          = "date"
		contentLengthHeader = "Content-Length"
		contentTypeHeader   = "Content-Type"
	)

	signer := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString([]byte(priv.Public().(ed25519.PublicKey))),
			Headers: []string{
				"(request-target)",
				"date",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: priv,
		Opts:     crypto.Hash(0),
	}

	buf := bytes.NewBuffer([]byte{})
	err = json.NewEncoder(buf).Encode(data)
	body := buf.Bytes()
	if err != nil {
		log.Fatalf("failed to marshal attested transaction body: %w", err)
	}
	apiBase := os.Getenv("NITRO_API_BASE")

	// Create a request and set the headers we require for signing. The Digest header is added
	// during the signing call and the request.Host is set during the new request creation so,
	// we don't need to explicitly set them here.
	req, err := http.NewRequest(http.MethodPost, apiBase+path, buf)
	if err != nil {
		log.Fatalf("failed to create request to sign: %w", err)
	}
	req.Header.Set(dateHeader, time.Now().Format(time.RFC1123))
	req.Header.Set(contentLengthHeader, fmt.Sprintf("%d", len(body)))
	req.Header.Set(contentTypeHeader, "application/json")

	// http sign the request
	err = signer.SignRequest(req)
	if err != nil {
		log.Fatalf("failed to sign request: %w", err)
	}

	httpClient, err := client.NewWithHTTPClient(apiBase, "", &http.Client{
		Timeout: time.Second * 60,
	})
	if err != nil {
		log.Fatalf("failed to create http client: %w", err)
	}
	resp, err := httpClient.Do(ctx, req, nil)
	if err != nil {
		log.Fatalf("failed to submit http request: %w", err)
	}
	return resp
}
