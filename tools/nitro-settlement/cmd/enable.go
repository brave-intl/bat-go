package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/nitro-settlement/internal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add enable to our root settlement-cli command
	rootCmd.AddCommand(enableCmd)

	// configurations for enable command

	// enable-file - the enable configuration file
	enableCmd.Flags().String(shareKey, "", "the operators shamir share in hex")
	viper.BindPFlag(shareKey, enableCmd.Flags().Lookup(shareKey))

	// kms-key - the kms key arn to use to encrypt
	enableCmd.Flags().String(kmsKeyKey, "", "the kms key to use to encrypt with")
	viper.BindPFlag(enableKmsKeyKey, enableCmd.Flags().Lookup(kmsKeyKey))

	// s3-bucket - the s3 bucket to store the enable data
	enableCmd.Flags().String(s3BucketKey, "", "the s3 bucket to upload the enable file to")
	viper.BindPFlag(enableS3BucketKey, enableCmd.Flags().Lookup(s3BucketKey))
}

// enableCmd is the nitro settlements prepare command, which loads transactions into workflow
var (
	enableCmd = &cobra.Command{
		Use:   "enable",
		Short: "enable enclave to retrieve bootsrap config",
		Run:   cmdutils.Perform("enable settlement", enableRun),
	}
	shareKey          = "share"
	enableKmsKeyKey   = "enable-kms-key"
	enableS3BucketKey = "enable-s3-bucket"
)

// enableRun - main entrypoint for the `enable` subcommand
func enableRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "enable").Info().Msg("performing enable...")

	logging.Logger(ctx, "enable").Info().
		Str(shareKey, viper.GetString(shareKey)).
		Str(enableKmsKeyKey, viper.GetString(enableKmsKeyKey)).
		Str(enableS3BucketKey, viper.GetString(enableS3BucketKey)).
		Msg("configuration")

	// make the config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return internal.LogAndError(ctx, err, "enable", "failed to load default aws config")
	}
	logging.Logger(ctx, "enable").Info().Msg("aws config loaded...")

	// get kms client from config
	kmsClient := kms.NewFromConfig(cfg)
	logging.Logger(ctx, "enable").Info().Msg("created kms client...")

	// list the key policies associated with the key
	keyPolicies, err := kmsClient.ListKeyPolicies(ctx, &kms.ListKeyPoliciesInput{
		KeyId: aws.String(viper.GetString(enableKmsKeyKey)),
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "enable", "failed to get key policy")
	}
	logging.Logger(ctx, "enable").Info().Msg("aws key policy downloaded...")

	for _, policy := range keyPolicies.PolicyNames {
		// get the key policy associated with the key, prompt user to continue or not
		keyPolicy, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId:      aws.String(viper.GetString(enableKmsKeyKey)),
			PolicyName: aws.String(policy),
		})
		if err != nil {
			return internal.LogAndError(ctx, err, "enable", "failed to get key policy")
		}
		logging.Logger(ctx, "enable").Info().Msg("aws key policy downloaded...")

		// print out the policy name, principal and conditions of who can decrypt
		var p = new(internal.KeyPolicy)
		if err := json.Unmarshal([]byte(*keyPolicy.Policy), p); err != nil {
			return internal.LogAndError(ctx, err, "enable", "failed to parse key policy")
		}

		for _, statement := range p.Statement {
			if statement.Effect == "Allow" && strings.Contains(strings.Join(statement.Action, "|"), "Decrypt") {
				conditions, err := json.MarshalIndent(statement.Condition, "", "\t")
				if err != nil {
					return internal.LogAndError(ctx, err, "enable", "failed to parse key policy conditions")
				}
				fmt.Printf("\nPrincipal: %+v \n\tConditions: %s\n", statement.Principal.AWS, string(conditions))
			}
		}
	}

	var input = "no"
	fmt.Printf("Would you like to continue? (yes|no) ")
	fmt.Scanln(&input)

	if strings.EqualFold(input, "no") {
		logging.Logger(ctx, "enable").Info().Msg("ending enable process")
		os.Exit(0)
	}

	// perform encryption of the operator's shamir share
	out, err := kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(viper.GetString(enableKmsKeyKey)),
		Plaintext: []byte(viper.GetString(shareKey)),
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "enable", "failed to encrypt enable file")
	}
	logging.Logger(ctx, "enable").Info().Msg("enable file encrypted...")

	s3Client := s3.NewFromConfig(cfg)
	logging.Logger(ctx, "enable").Info().Msg("created s3 client...")

	h := md5.New()
	h.Write(out.CiphertextBlob)

	// put the ciphertext of the operator's share in s3.  the enclave is the only thing that can
	// decrypt this, and waits until it has threshold shares to combine and decrypt the secrets
	// living in bootstrap file.
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(viper.GetString(enableS3BucketKey)),
		Key: aws.String(
			fmt.Sprintf("operator-share_%s.json", time.Now().Format(time.RFC3339))),
		Body:                      bytes.NewBuffer(out.CiphertextBlob),
		ContentMD5:                aws.String(base64.StdEncoding.EncodeToString(h.Sum(nil))),
		ObjectLockLegalHoldStatus: s3types.ObjectLockLegalHoldStatusOn,
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "enable", "failed to upload enable file")
	}
	logging.Logger(ctx, "enable").Info().Msg("enable file uploaded...")

	logging.Logger(ctx, "enable").Info().
		Msg("completed enable of payments service")

	return nil
}
