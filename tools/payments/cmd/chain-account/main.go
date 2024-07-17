/*
chain-account allows payment operators to add addresses to be used for on-chain payouts.

Usage:

chain-account [flags] [command] args...

The flags are:

	-pr
		Location on file system of the original prepared report
	-v
		verbose logging enabled
	-k
		Location on file system of the operators private ED25519 signing key in PEM format.
	-e
		The environment to which the operator is sending approval for transactions.
*/
package main

import (
	"bytes"
	"context"
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

	env := flag.String(
		"e", "local",
		"the environment to which the tool will interact")

	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	pcr2 := flag.String(
		"pcr2", "", "the hex PCR2 value for this enclave")

	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		log.Fatal("Expected chain id and action arguments")
	}

	switch args[0] {
	case "solana":
		switch args[1] {
		case "generate":
			generateSolanaAddress(ctx, *operatorKey, *env, pcr2, *verbose)
		case "approve":
			if len(args) < 3 {
				log.Fatal("Expected public key argument for approval")
			}
			pubKey := args[2]
			approveSolanaAddress(ctx, pubKey, *operatorKey, *env, pcr2, *verbose)
		default:
			log.Fatal("unrecognized solana command")
		}
	default:
		log.Fatal("unrecognized chain id")
	}

}

func generateSolanaAddress(ctx context.Context, key, env string, pcr2 *string, verbose bool) {
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

	signer := httpsignature.GetEd25519RequestSignator(priv)

	buf := bytes.NewBuffer([]byte{})
	buf.WriteString(key)
	body := buf.Bytes()
	if err != nil {
		log.Fatalf("failed to marshal attested transaction body: %w", err)
	}
	apiBase := os.Getenv("NITRO_API_BASE")

	// Create a request and set the headers we require for signing. The Digest header is added
	// during the signing call and the request.Host is set during the new request creation so,
	// we don't need to explicitly set them here.
	req, err := http.NewRequest(http.MethodPost, apiBase+"/v1/payments/generatesol", buf)
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
	sp, verifier, err := paymentscli.NewNitroVerifier(pcr2)
	if err != nil {
		log.Fatalln(err)
	}

	valid, err := sp.VerifyResponse(verifier, resp)
	if err != nil {
		log.Fatalln(err)
	}
	if !valid {
		log.Fatalln("http signature was not valid, nitro attestation failed")
	}
	log.Printf("%+v", resp)

	if verbose {
		log.Println("generatesol command complete")
	}
}

func approveSolanaAddress(ctx context.Context, pubKey, key, env string, pcr2 *string, verbose bool) {
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

	signer := httpsignature.GetEd25519RequestSignator(priv)

	chainAddress := payments.AddressApprovalRequest{
		Address: pubKey,
	}
	buf := bytes.NewBuffer([]byte{})
	err = json.NewEncoder(buf).Encode(chainAddress)
	body := buf.Bytes()
	if err != nil {
		log.Fatalf("failed to marshal attested transaction body: %w", err)
	}
	apiBase := os.Getenv("NITRO_API_BASE")

	// Create a request and set the headers we require for signing. The Digest header is added
	// during the signing call and the request.Host is set during the new request creation so,
	// we don't need to explicitly set them here.
	req, err := http.NewRequest(http.MethodPost, apiBase+"/v1/payments/approvesol", buf)
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
	sp, verifier, err := paymentscli.NewNitroVerifier(pcr2)
	if err != nil {
		log.Fatalln(err)
	}

	valid, err := sp.VerifyResponse(verifier, resp)
	if err != nil {
		log.Fatalln(err)
	}
	if !valid {
		log.Fatalln("http signature was not valid, nitro attestation failed")
	}
	log.Printf("%+v", resp)

	if verbose {
		log.Println("approvesol command complete")
	}
}
