/*
Configure encrypts a configuration file for consumption by the payments service, the output of which
is then uploaded to s3 and consumed by the payments service.

Configure takes as parameters the public key output from the configure command, and a configuration file.

Usage:

configure [flags] file [files]

The flags are:

	-k
		The public key of the payments service (output from create command)
	-b
		The s3 uri to upload the configuration to

The arguments are configuration files which are to be encrypted.
*/
package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"flag"
	"io"
	"log"
	"os"

	"filippo.io/age"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws"
)

func main() {
	// command line flags
	publicKey := flag.String(
		"k", "",
		"the public key of the payment service (from create command)")
	s3Bucket := flag.String(
		"b", "",
		"the s3 bucket to upload to")
	s3Object := flag.String(
		"o", "",
		"the s3 object name for output")
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

	// make the config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load default aws config: %v", err)
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

		s3Client := s3.NewFromConfig(cfg)
		h := md5.New()
		h.Write(buf.Bytes())

		// put the enclave configuration up in s3
		_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:                    aws.String(*s3Bucket),
			Key:                       aws.String(*s3Object),
			Body:                      buf,
			ContentMD5:                aws.String(base64.StdEncoding.EncodeToString(h.Sum(nil))),
			ObjectLockLegalHoldStatus: s3types.ObjectLockLegalHoldStatusOn,
		})
		if err != nil {
			log.Fatalf("failed to upload configuration: %v", err)
		}
		log.Printf("payments enclave to use configuration: %s/%s\n", *s3Bucket, *s3Object)
	}

	if *verbose {
		log.Println("completed configure.")
	}
}
