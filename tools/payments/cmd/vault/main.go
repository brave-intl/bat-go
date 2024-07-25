/*
Vault instructs the enclave to generate and approve a random asymmetric key
pair, break the private key into operator shares, return the shares after they
have been encrypted with provided operator keys, and then discard the private
key so that the operators are needed to access it again.

# Create takes as parameters a set of operator pubkeys and a threshold

Usage:

vault [flags] create
vault [flags] verify

The flags are:

	-t
		The Shamir share threshold to reconstitute the private key
	-pcr2
		The public key for the vault private key that is being approved.
	-s
		The encrypted share file for the verifying operator. Only needed for verify subcommand
	-p
		The vault public key. Only needed for verify subcommand
	-k
		Location on file system of the operators private ED25519 signing key in PEM format. Only needed for verify subcommand
*/
package main

import (
	"bytes"
	"context"
	"crypto"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	client "github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/payments"
	paymentscli "github.com/brave-intl/payments-service/tools/payments"
)

func main() {
	ctx := context.Background()

	// command line flags
	threshold := flag.Int("t", 2, "the threshold for Shamir shares to reconstitute the private key")
	pcr2 := flag.String("pcr2", "", "the hex PCR2 value for this enclave")
	vaultPublicKey := flag.String("p", "", "the vault public key. only needed for verify subcommand")
	shareFile := flag.String("s", "", "the encrypted share file for the verifying operator. only needed for verify subcommand")
	operatorKeyFile := flag.String(
		"k",
		"test/private.pem",
		"the operator's key file location (ed25519 private key) in PEM format. only needed for verify subcommand",
	)
	verbose := flag.Bool("v", false, "view verbose logging")

	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("expected subcommand (create or approve)")
	}

	if *verbose {
		// print out the configuration
		log.Printf("Threshold: %d\n", *threshold)
	}

	switch args[0] {
	case "create":
		resp := doRequest(
			ctx,
			"/v1/payments/vault/create",
			pcr2,
			payments.CreateVaultRequest{Threshold: *threshold},
		)
		defer resp.Body.Close()

		vaultResp := payments.CreateVaultResponseWrapper{}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("failed to read response json: %w", err)
		}
		err = json.Unmarshal(body, &vaultResp)
		if err != nil {
			log.Fatalf("failed to unmarshal response json: %w", err)
		}

		for _, share := range vaultResp.Data.Shares {
			fname := fmt.Sprintf("share-%s-%s.enc", share.Name, vaultResp.Data.PublicKey)
			err = os.WriteFile(fname, share.Material, 0644)
			if err != nil {
				log.Fatalf("failed to write share file: %w", err)
			}
			log.Printf("Wrote file for %s to %s", share.Name, fname)
		}
		log.Printf("Generated Public Key: %s", vaultResp.Data.PublicKey)
	case "verify":
		priv, err := paymentscli.GetOperatorPrivateKey(*operatorKeyFile)
		if err != nil {
			log.Fatalf("failed to open operator key file: %v\n", err.Error())
		}

		identity, err := agessh.NewEd25519Identity(priv)
		if err != nil {
			log.Fatalf("failed to parse private key as identity: %v", err)
		}

		sf, err := os.Open(*shareFile)
		if err != nil {
			log.Fatalf("failed to open share file: %v", err)
		}

		r, err := age.Decrypt(sf, identity)
		if err != nil {
			log.Fatalf("failed to decrypt share file: %v", err)
		}

		shareVal := &bytes.Buffer{}
		if _, err := io.Copy(shareVal, r); err != nil {
			log.Fatalf("failed to read encrypted share file: %v", err)
		}
		s := shareVal.Bytes()
		// We don't actually need this value, but we do want to make sure that we are able to
		// decrypt it with the operator key as validation that the share was created with the
		// expected public key.
		if len(s) < 1 {
			log.Fatal("share is empty")
		}

		resp := doRequest(
			ctx,
			"/v1/payments/vault/verify",
			pcr2,
			payments.VerifyVaultRequest{
				Threshold: *threshold,
				PublicKey: *vaultPublicKey,
			},
		)
		vaultResp := payments.VerifyVaultResponseWrapper{}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("failed to read response json: %w", err)
		}
		err = json.Unmarshal(body, &vaultResp)
		if err != nil {
			log.Fatalf("failed to unmarshal response json: %w", err)
		}
		if vaultResp.Data.PublicKey != *vaultPublicKey {
			log.Fatalf(
				"public key mismatch between what was provided and what is stored in the service. ours: %s theirs: %s",
				*vaultPublicKey,
				vaultResp.Data.PublicKey,
			)
		}
		if vaultResp.Data.Threshold != *threshold {
			log.Fatalf(
				"threshold mismatch between what was provided and what is stored in the service. ours: %s theirs: %s",
				*threshold,
				vaultResp.Data.Threshold,
			)
		}
		log.Printf("Result: %s", body)
		log.Print("Results match expected data. Verification complete.")
	default:
		log.Fatal("unrecognized subcommand. options are create and approve")
	}

	if *verbose {
		log.Println("completed create.")
	}
}

func doRequest(
	ctx context.Context,
	path string,
	pcr2 *string,
	data interface{},
) *http.Response {
	buf := bytes.NewBuffer([]byte{})
	err := json.NewEncoder(buf).Encode(data)
	if err != nil {
		log.Fatalf("failed to marshal attested transaction body: %w", err)
	}
	apiBase := os.Getenv("NITRO_API_BASE")

	req, err := http.NewRequest(http.MethodPost, apiBase+path, buf)
	if err != nil {
		log.Fatalf("failed to create request to sign: %w", err)
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

	valid, err := sp.VerifyResponse(verifier, crypto.Hash(0), resp)
	if err != nil {
		log.Fatalln(err)
	}
	if !valid {
		log.Fatalln("http signature was not valid, nitro attestation failed")
	}
	return resp
}
