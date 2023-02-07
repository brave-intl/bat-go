package cmd

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/nitro-settlement/internal"
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

	// assume-role - the role to assume (needs iam ability to encrypt/s3 put
	bootstrapCmd.Flags().String(assumeRoleKey, "", "the role to assume which allows encrypt/upload")
	viper.BindPFlag(assumeRoleKey, bootstrapCmd.Flags().Lookup(assumeRoleKey))

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
	assumeRoleKey    = "assume-role"
	kmsKeyKey        = "kms-key"
	s3BucketKey      = "s3-bucket"
)

// bootstrapRun - main entrypoint for the `bootstrap` subcommand
func bootstrapRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "bootstrap").Info().Msg("performing bootstrap...")

	logging.Logger(ctx, "bootstrap").Info().
		Str(bootstrapFileKey, viper.GetString(bootstrapFileKey)).
		Str(assumeRoleKey, viper.GetString(assumeRoleKey)).
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

	// make the config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to load default aws config")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("aws config loaded...")

	// setup creds for role we need to assume
	stsSvc := sts.NewFromConfig(cfg)
	creds := stscreds.NewAssumeRoleProvider(stsSvc, viper.GetString(assumeRoleKey))
	cfg.Credentials = aws.NewCredentialsCache(creds)

	// get kms client from config - assumed role
	kmsClient := kms.NewFromConfig(cfg)
	logging.Logger(ctx, "bootstrap").Info().Msg("created kms client...")

	// perform encryption of the bootstrap file
	out, err := kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(viper.GetString(kmsKeyKey)),
		Plaintext: bootstrap,
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to encrypt bootstrap file")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap file encrypted...")

	s3Client := s3.NewFromConfig(cfg)
	logging.Logger(ctx, "bootstrap").Info().Msg("created s3 client...")

	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(viper.GetString(s3BucketKey)),
		Key:    aws.String("bootstrap.json"),
		Body:   bytes.NewBuffer(out.CiphertextBlob),
	})
	if err != nil {
		return internal.LogAndError(ctx, err, "bootstrap", "failed to upload bootstrap file")
	}
	logging.Logger(ctx, "bootstrap").Info().Msg("bootstrap file uploaded...")

	logging.Logger(ctx, "bootstrap").Info().
		Msg("completed bootstrapping of payments service")

	return nil
}
