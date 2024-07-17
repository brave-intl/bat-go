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
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	client "github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/payments"
	paymentscli "github.com/brave-intl/bat-go/tools/payments"
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
		"",
		"the operator's key file location (ed25519 private key) in PEM format.",
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

	priv, err := paymentscli.GetOperatorPrivateKey(*operatorKeyFile)
	if err != nil {
		log.Fatalf("failed to open operator key file: %v\n", err.Error())
	}

	switch args[0] {
	case "create":
		resp, err := doRequest(
			ctx,
			"/v1/payments/vault/create",
			priv,
			pcr2,
			payments.CreateVaultRequest{Threshold: *threshold},
		)
		if err != nil {
			log.Fatalf("failed to send the vault create request: %v\n", err)
		}
		defer resp.Body.Close()

		vaultResp := payments.CreateVaultResponseWrapper{}
		body, err := io.ReadAll(resp.Body)
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

		resp, err := doRequest(
			ctx,
			"/v1/payments/vault/verify",
			priv,
			pcr2,
			payments.VerifyVaultRequest{
				Threshold: *threshold,
				PublicKey: *vaultPublicKey,
			},
		)
		if err != nil {
			log.Fatalf("failed to send the vault verify request: %v\n", err)
		}
		vaultResp := payments.VerifyVaultResponseWrapper{}
		body, err := io.ReadAll(resp.Body)
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
	key ed25519.PrivateKey,
	pcr2 *string,
	data interface{},
) (*http.Response, error) {
	buf := bytes.NewBuffer([]byte{})
	err := json.NewEncoder(buf).Encode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attested transaction body: %w", err)
	}
	apiBase := os.Getenv("NITRO_API_BASE")

	req, err := http.NewRequest(http.MethodPost, apiBase+path, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to sign: %w", err)
	}
	req.Header.Set("date", time.Now().Format(time.RFC1123))
	req.Header.Set("content-length", fmt.Sprintf("%d", buf.Len()))
	req.Header.Set("content-type", "application/json")

	signator := httpsignature.GetEd25519RequestSignator(key)
	err = signator.SignRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to sign HTTP request: %w", err)
	}

	httpClient, err := client.NewWithHTTPClient(apiBase, "", &http.Client{
		Timeout: time.Second * 60,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create http client: %w", err)
	}
	resp, err := httpClient.Do(ctx, req, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit http request: %w", err)
	}

	sp, verifier, err := paymentscli.NewNitroVerifier(pcr2)
	if err != nil {
		return nil, err
	}

	valid, err := sp.VerifyResponse(verifier, resp)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("http signature was not valid, nitro attestation failed")
	}
	return resp, nil
}
