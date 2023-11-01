/*
Bootstrap takes the provided operator shamir key share and encrypts the key with
the provided KMS encryption key (that only the enclave can decrypt with) and then
uploads the ciphertext to s3 for the enclave to download

Bootstrap takes as parameters the operator share, kms key arn and s3 uri.

Usage:

bootstrap [flags]

The flags are:

	-s
		The operator's Shamir key share from the create command
	-u
		The enclave services' base uri to get the key id from
	-b
		The S3 URI to upload the ciphertext to
*/
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	paymentslib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/tools/payments"
)

func main() {
	ctx := context.Background()
	// command line flags
	env := flag.String(
		"e", "local",
		"the environment to which the tool will interact")
	s := flag.String(
		"s", "",
		"the operators shamir key share")
	b := flag.String(
		"b", "", "the s3 bucket to upload ciphertext to")
	pcr2 := flag.String(
		"pcr2", "", "the hex PCR2 value for this enclave")
	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Environment: %s\n", *env)
		log.Printf("Operator Shamir Share: %s\n", *s)
		log.Printf("S3 Bucket URI: %s\n", *b)
	}

	enclaveBaseURI, ok := paymentslib.APIBase[*env]
	if !ok {
		log.Fatalln("Invalid env:", *env)
	}

	// get the info endpoint to key kms arn
	resp, err := http.Get(enclaveBaseURI + "/v1/payments/info")
	if err != nil {
		log.Fatalln(err)
	}

	sp, verifier, err := payments.NewNitroVerifier(pcr2)
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

	data := make(map[string]string)
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		log.Fatalln(err)
	}
	resp.Body.Close()

	encryptKeyArn := data["encryptionKeyArn"]

	if data["environment"] != *env {
		log.Fatalf("environments do not match!! payments service environment: %s", data["environment"])
	}

	// make the config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load default aws config: %v", err)
	}

	// get kms client from config
	kmsClient := kms.NewFromConfig(cfg)

	// list the key policies associated with the key
	keyPolicies, err := kmsClient.ListKeyPolicies(ctx, &kms.ListKeyPoliciesInput{
		KeyId: aws.String(encryptKeyArn),
	})
	if err != nil {
		log.Fatalf("failed to get key policy: %v", err)
	}

	for _, policy := range keyPolicies.PolicyNames {
		// get the key policy associated with the key, prompt user to continue or not
		keyPolicy, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId:      aws.String(encryptKeyArn),
			PolicyName: aws.String(policy),
		})
		if err != nil {
			log.Fatalf("failed to get key policy: %v", err)
		}

		// print out the policy name, principal and conditions of who can decrypt
		var p = new(payments.KeyPolicy)
		if err := json.Unmarshal([]byte(*keyPolicy.Policy), p); err != nil {
			log.Fatalf("failed to parse key policy: %v", err)
		}

		for _, statement := range p.Statement {
			if statement.Effect == "Allow" && strings.Contains(fmt.Sprintf("%+v", statement.Action), "Decrypt") {
				conditions, err := json.MarshalIndent(statement.Condition, "", "\t")
				if err != nil {
					log.Fatalf("failed to parse key policy conditions: %v", err)
				}
				if *verbose {
					log.Printf("\nPrincipal: %+v \n\tConditions: %s\n", statement.Principal.AWS, string(conditions))
				}
			}
		}
	}

	// perform encryption of the operator's shamir share
	out, err := kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(encryptKeyArn),
		Plaintext: []byte(*s),
	})
	if err != nil {
		log.Fatalf("failed to encrypt operator key share: %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	h := md5.New()
	h.Write(out.CiphertextBlob)

	// put the ciphertext of the operator's share in s3.  the enclave is the only thing that can
	// decrypt this, and waits until it has threshold shares to combine and decrypt the secrets
	// living in bootstrap file.
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(*b),
		Key: aws.String(
			fmt.Sprintf("%s/operator-share_%s.json", *pcr2, time.Now().Format(time.RFC3339))),
		Body:                      bytes.NewBuffer(out.CiphertextBlob),
		ContentMD5:                aws.String(base64.StdEncoding.EncodeToString(h.Sum(nil))),
		ObjectLockLegalHoldStatus: s3types.ObjectLockLegalHoldStatusOn,
	})
	if err != nil {
		log.Fatalf("failed to encrypt operator key share: %v", err)
	}

	if *verbose {
		log.Println("completed bootstrap.")
	}
}
