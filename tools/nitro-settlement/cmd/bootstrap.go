package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/nitro-settlement/internal"
	"github.com/hashicorp/vault/shamir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add bootstrap to our root settlement-cli command
	rootCmd.AddCommand(bootstrapCmd)

	// configurations for bootstrap command

	// bootstrap-file - the bootstrap configuration file
	bootstrapCmd.Flags().String(bootstrapFileKey, "", "the location of bootstrap file")
	viper.BindPFlag(bootstrapFileKey, bootstrapCmd.Flags().Lookup(bootstrapFileKey))

	// kms-key - the kms key arn to use to encrypt
	bootstrapCmd.Flags().String(kmsKeyKey, "", "the kms key to use to encrypt with")
	viper.BindPFlag(kmsKeyKey, bootstrapCmd.Flags().Lookup(kmsKeyKey))

	// s3-bucket - the s3 bucket to store the bootstrap data
	bootstrapCmd.Flags().String(s3BucketKey, "", "the s3 bucket to upload the bootstrap file to")
	viper.BindPFlag(s3BucketKey, bootstrapCmd.Flags().Lookup(s3BucketKey))
}

// bootstrapCmd is the nitro settlements prepare command, which loads transactions into workflow
var (
	bootstrapCmd = &cobra.Command{
		Use:   "bootstrap",
		Short: "bootstrap transactions for settlement",
		Run:   cmdutils.Perform("bootstrap settlement", bootstrapRun),
	}
	bootstrapFileKey = "bootstrap-file"
	kmsKeyKey        = "kms-key"
	s3BucketKey      = "s3-bucket"
)

// bootstrapRun - main entrypoint for the `bootstrap` subcommand
func bootstrapRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "bootstrap").Info().Msg("performing bootstrap...")

	logging.Logger(ctx, "bootstrap").Info().
		Str(bootstrapFileKey, viper.GetString(bootstrapFileKey)).
		Str(kmsKeyKey, viper.GetString(kmsKeyKey)).
		Str(s3BucketKey, viper.GetString(s3BucketKey)).
		Msg("configuration")

	// read the bootstrap file from disk
	bsFile, err := os.Open(viper.GetString(bootstrapFileKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to open the bootstrap file")
	}
	defer bsFile.Close()
	bootstrap, err := io.ReadAll(bsFile)
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to read the bootstrap file")
	}

	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap file read...")

	// we need to come up with a random key to encrypt the file with
	key := make([]byte, 32)

	_, err = rand.Read(key)
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to create encryption key")
	}

	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap created encryption key...")

	// we need to derive shamir shares to combine to get the random key we made
	shares, err := shamir.Split(key, 5, 2)
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to create key shares for operators")
	}

	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap created key shares for operators...")

	// we need to output the operator shares to reconstitute the random key (in enclave)
	for i, v := range shares {
		fmt.Printf("\t!!! OPERATOR SHARE %d: %s\n", i, hex.EncodeToString(v))
	}

	// we need to encrypt bootstrap with this random key
	var k [32]byte
	copy(k[:], key)

	ciphertext, nonce, err := cryptography.EncryptMessage(k, bootstrap)
	bootstrapCiphertext := append(nonce[:], ciphertext...) // put nonce at start of ciphertext

	// make the config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to load default aws config")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("aws config loaded...")

	// get kms client from config
	kmsClient := kms.NewFromConfig(cfg)
	logging.Logger(ctx, "bootstrap").Info().Msg("created kms client...")

	// list the key policies associated with the key
	keyPolicies, err := kmsClient.ListKeyPolicies(ctx, &kms.ListKeyPoliciesInput{
		KeyId: aws.String(viper.GetString(kmsKeyKey)),
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to get key policy")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("aws key policy downloaded...")

	for _, policy := range keyPolicies.PolicyNames {
		// get the key policy associated with the key, prompt user to continue or not
		keyPolicy, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId:      aws.String(viper.GetString(kmsKeyKey)),
			PolicyName: aws.String(policy),
		})
		if err != nil {
			return internal.LogAndError(ctx, err, "bootstrap", "failed to get key policy")
		}
		logging.Logger(ctx, "bootstrap").Info().Msg("aws key policy downloaded...")

		// print out the policy name, principal and conditions of who can decrypt
		var p = new(internal.KeyPolicy)
		if err := json.Unmarshal([]byte(*keyPolicy.Policy), p); err != nil {
			return internal.LogAndError(ctx, err, "bootstrap", "failed to parse key policy")
		}

		for _, statement := range p.Statement {
			if statement.Effect == "Allow" && strings.Contains(strings.Join(statement.Action, "|"), "Decrypt") {
				conditions, err := json.MarshalIndent(statement.Condition, "", "\t")
				if err != nil {
					return internal.LogAndError(ctx, err, "bootstrap", "failed to parse key policy conditions")
				}
				fmt.Printf("\nPrincipal: %+v \n\tConditions: %s\n", statement.Principal.AWS, string(conditions))
				// TODO: validate with what we got back from enclave for measurements
			}
		}
	}

	// perform encryption of the bootstrap file
	out, err := kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(viper.GetString(kmsKeyKey)),
		Plaintext: bootstrapCiphertext,
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to encrypt bootstrap file")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap file encrypted...")

	s3Client := s3.NewFromConfig(cfg)
	logging.Logger(ctx, "bootstrap").Info().Msg("created s3 client...")

	h := md5.New()
	h.Write(out.CiphertextBlob)

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:                    aws.String(viper.GetString(s3BucketKey)),
		Key:                       aws.String("bootstrap.json"),
		Body:                      bytes.NewBuffer(out.CiphertextBlob),
		ContentMD5:                aws.String(base64.StdEncoding.EncodeToString(h.Sum(nil))),
		ObjectLockLegalHoldStatus: s3types.ObjectLockLegalHoldStatusOn,
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to upload bootstrap file")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap file uploaded...")

	logging.Logger(ctx, "bootstrap").Info().
		Msg("completed bootstrapping of payments service")

	return nil
}
