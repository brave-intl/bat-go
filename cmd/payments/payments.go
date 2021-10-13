package payments

import (

	// pprof imports
	_ "net/http/pprof"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// variables that will be build time configurations
	// configURL - the url location of the configuration for the service, currently supports (file:// and s3://)
	configURL string
	// keyARN - the aws arn of the decryption key to decrypt the configuration file
	keyARN string
	// environment - the environment this service is running in
	environment string
)

func init() {
	// add grpc and rest commands
	paymentsCmd.AddCommand(grpcCmd)
	paymentsCmd.AddCommand(restCmd)

	// add this command as a serve subcommand
	cmd.ServeCmd.AddCommand(paymentsCmd)

	// setup the flags

	// environment will be compiled into the application, if absent we are to use the flags
	if environment == "" {
		// --environment - location of configuration
		paymentsCmd.PersistentFlags().StringVar(&environment, "environment", "",
			"the environment we are running in - typically compile time var")
		cmd.Must(viper.BindPFlag("environment", paymentsCmd.PersistentFlags().Lookup("environment")))
		cmd.Must(viper.BindEnv("environment", "ENV"))
	}

	// configURL will be compiled into the application, if absent we are to use the flags
	if configURL == "" {
		// --config-url - location of configuration
		paymentsCmd.PersistentFlags().StringVar(&configURL, "config-url", "",
			"the full location url of the configuration")
		cmd.Must(viper.BindPFlag("config-url", paymentsCmd.PersistentFlags().Lookup("config-url")))
		cmd.Must(viper.BindEnv("config-url", "CONFIG_URL"))
	}

	// keyARN will be compiled into the application, if absent we are to use the flags
	if keyARN == "" {
		// --key-arn - arn of the encryption key used
		paymentsCmd.PersistentFlags().StringVar(&keyARN, "key-arn", "",
			"the aws key arn used to decrypt configuration file")
		cmd.Must(viper.BindPFlag("key-arn", paymentsCmd.PersistentFlags().Lookup("key-arn")))
		cmd.Must(viper.BindEnv("key-arn", "KEY_ARN"))
	}

	// --batch-sign-keypair - keypair used for signing
	paymentsCmd.PersistentFlags().String("batch-sign-keypair", "",
		"the key pair used to sign batches")
	cmd.Must(viper.BindPFlag("batch-sign-keypair", paymentsCmd.PersistentFlags().Lookup("batch-sign-keypair")))
	cmd.Must(viper.BindEnv("batch-sign-keypair", "BATCH_SIGN_KEYPAIR"))

	// --cert - file location of the certificate
	paymentsCmd.PersistentFlags().String("cert", "",
		"the file location of the cert")
	cmd.Must(viper.BindPFlag("cert", paymentsCmd.PersistentFlags().Lookup("cert")))
	cmd.Must(viper.BindEnv("cert", "CERT"))

	// --cert-key - file location of the certificate key
	paymentsCmd.PersistentFlags().String("cert-key", "",
		"the file location of the cert key")
	cmd.Must(viper.BindPFlag("cert-key", paymentsCmd.PersistentFlags().Lookup("cert-key")))
	cmd.Must(viper.BindEnv("cert-key", "CERT_KEY"))

}

var (
	paymentsCmd = &cobra.Command{
		Use:   "payments",
		Short: "provides payments micro-service entrypoint",
	}

	restCmd = &cobra.Command{
		Use:   "rest",
		Short: "provides REST api services",
		Run:   RestRun,
	}

	grpcCmd = &cobra.Command{
		Use:   "grpc",
		Short: "provides gRPC api services",
		Run:   GRPCRun,
	}
)
