/*
Configure encrypts a configuration file for consumption by the payments service, the output of which
is then uploaded to s3 and consumed by the payments service.

Configure takes as parameters the public key output from the configure command, and a configuration file.

Usage:

configure [flags] file [files]

The flags are:

	-k
		The public key of the payments service (output from create command)
	-u
		The enclave services' base uri to get the key id from
	-b
		The s3 uri to upload the configuration to

The arguments are configuration files which are to be encrypted.
*/
package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"filippo.io/age"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
)

func main() {
	// command line flags
	publicKey := flag.String(
		"k", "",
		"the public key of the payment service (from create command)")
	enclaveBaseURI := flag.String(
		"u", "",
		"the enclave base URI")
	s3Bucket := flag.String(
		"b", "",
		"the s3 bucket to upload to")
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

	// get the info endpoint to key kms arn
	resp, err := http.Get(enclaveBaseURI + "/v1/info")
	if err != nil {
		log.Fatalln(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln(err)
	}
	resp.Body.Close()

	data := make(map[string]string)
	err := json.Unmarshal(body, data)

	encryptKeyARN = data["encryptionKeyArn"]
	// make the config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load default aws config: %v", err)
	}

	// get kms client from config
	kmsClient := kms.NewFromConfig(cfg)

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

		// holds ciphertext of configuration
		buf := bytes.NewBuffer([]byte{})

		w, err := age.Encrypt(buf, recipient)
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

		// perform encryption of the operator's shamir share
		out, err := kmsClient.Encrypt(ctx, &kms.EncryptInput{
			KeyId:     aws.String(encryptKeyArn),
			Plaintext: buf.Bytes(),
		})
		if err != nil {
			log.Fatalf("failed to encrypt configuration: %v", err)
		}

		s3Client := s3.NewFromConfig(cfg)
		h := md5.New()
		h.Write(out.CiphertextBlob)
		configObjectName := fmt.Sprintf("configuration_%s.json", time.Now().Format(time.RFC3339))),

		// put the enclave configuration up in s3
		_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(*b),
			Key: aws.String(configObjectName),
			Body:                      bytes.NewBuffer(out.CiphertextBlob),
			ContentMD5:                aws.String(base64.StdEncoding.EncodeToString(h.Sum(nil))),
			ObjectLockLegalHoldStatus: s3types.ObjectLockLegalHoldStatusOn,
		})
		if err != nil {
			log.Fatalf("failed to encrypt configuration share: %v", err)
		}
		log.Printf("payments enclave to use configuration: %s\n", configObjectName)
	}

	if *verbose {
		log.Println("completed configure.")
	}
}
