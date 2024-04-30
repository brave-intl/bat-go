/*
disaster is a disaster recovery command that will download the encrypted secrets file from S3 and decrypt it using locally-possessed shares.

Usage:

disaster [flags] [shareFiles...]

The flags are:

	-k
		Location on file system of the operators private ED25519 signing key in PEM format.
	-e
		The environment to which the operator is sending approval for transactions.
	-b
		The s3 from which to retrieve the configuration
	-f
		The filename to get from the s3 bucket
*/
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"filippo.io/age"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/hashicorp/vault/shamir"
)

func main() {
	ctx := context.Background()

	// command line flags
	operatorKey := flag.String(
		"k", "test/private.pem",
		"the operator's key file location (ed25519 private key) in PEM format")

	s3Bucket := flag.String(
		"b", "",
		"the s3 bucket to upload to")

	s3Object := flag.String(
		"o", "",
		"the s3 object name for output")

	env := flag.String(
		"e", "local",
		"the environment to which the tool will interact")

	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		log.Fatal("insufficient shares provided")
	}

	disaster(ctx, *operatorKey, *s3Bucket, *s3Object, args, *env, *verbose)
}

func disaster(ctx context.Context, key, bucket, file string, shareFilenames []string, env string, verbose bool) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load default aws config: %v", err)
	}
	s3Client := s3.NewFromConfig(cfg)
	output, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(file),
	})
	if err != nil {
		log.Fatalf("failed to upload configuration: %v", err)
	}

	bodyBytes, err := requestutils.Read(ctx, output.Body)
	if err != nil {
		log.Fatalf("failed to read response body: %w", err)
	}
	var shares [][]byte
	for _, f := range shareFilenames {
		dat, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("failed to read share file: %w", err)
		}
		shares = append(shares, dat)
	}
	recoveredData, err := decryptRecoveryData(bodyBytes, shares)
	if err != nil {
		log.Fatalf("failed to read attested recovery file: %v\n", err)
	}

	responseFile := "recovered-secrets.txt"
	err = os.WriteFile(responseFile, recoveredData, 0644)
	if err != nil {
		log.Fatalf("failed to write recovery data to file: %w", err)
	}

	if verbose {
		log.Println("disaster command complete")
	}
}

func decryptRecoveryData(encData []byte, shares [][]byte) ([]byte, error) {
	privateKey, err := shamir.Combine(shares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine keyShares: %w", err)
	}
	identity, err := age.ParseX25519Identity(string(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key bytes for secret decryption: %w", err)
	}
	dataReader, err := age.Decrypt(bytes.NewReader(encData), identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}
	data, err := ioutil.ReadAll(dataReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decrypted results: %w", err)
	}

	return data, nil
}
