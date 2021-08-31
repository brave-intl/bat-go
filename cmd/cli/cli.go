package cli

import (
	"context"

	"github.com/brave-intl/bat-go/cmd"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {

	// --payments-service - the dial address of the payments grpc service
	paymentsCliCmd.PersistentFlags().String("payments-service", "",
		"the location of the payments service")
	cmd.Must(viper.BindPFlag("payments-service", paymentsCliCmd.PersistentFlags().Lookup("payments-service")))
	cmd.Must(viper.BindEnv("payments-service", "PAYMENTS_SERVICE"))

	// --ca-cert - the ca certificate file location
	paymentsCliCmd.PersistentFlags().String("ca-cert", "",
		"the location of the ca certificate")
	cmd.Must(viper.BindPFlag("ca-cert", paymentsCliCmd.PersistentFlags().Lookup("ca-cert")))
	cmd.Must(viper.BindEnv("ca-cert", "CA_CERT"))

	// --custodian - the payout custodian
	paymentsCliCmd.PersistentFlags().String("custodian", "",
		"the custodian provider")
	cmd.Must(viper.BindPFlag("custodian", paymentsCliCmd.PersistentFlags().Lookup("custodian")))
	cmd.Must(viper.BindEnv("custodian", "CUSTODIAN"))

	// --payout-file - the payout transaction list file (json)
	paymentsCliCmd.PersistentFlags().String("payout-file", "",
		"the file of all of the transactions for the batch")
	cmd.Must(viper.BindPFlag("payout-file", paymentsCliCmd.PersistentFlags().Lookup("payout-file")))
	cmd.Must(viper.BindEnv("payout-file", "PAYOUT_FILE"))

	// --key-pair - the pem encoded keypair file for payments authorization
	paymentsCliCmd.PersistentFlags().String("key-pair", "",
		"the pem encoded key-pair file for payments authorization")
	cmd.Must(viper.BindPFlag("key-pair", paymentsCliCmd.PersistentFlags().Lookup("key-pair")))
	cmd.Must(viper.BindEnv("key-pair", "KEY_PAIR"))

	// --document-id - the qldb document id associated with the batch for authorize/submit
	paymentsCliCmd.PersistentFlags().String("document-id", "",
		"the qldb document id associated with the batch for payments authorization/submit")
	cmd.Must(viper.BindPFlag("document-id", paymentsCliCmd.PersistentFlags().Lookup("document-id")))
	cmd.Must(viper.BindEnv("document-id", "DOCUMENT_ID"))

	// add this command as a serve subcommand
	cmd.RootCmd.AddCommand(cliCmd)
	cliCmd.AddCommand(paymentsCliCmd)
}

var (
	cliCmd = &cobra.Command{
		Use:   "cli",
		Short: "provides generic cli entrypoint",
	}
	paymentsCliCmd = &cobra.Command{
		Use:   "payments",
		Short: "provides payments cli entrypoint",
		Run: func(cmd *cobra.Command, args []string) {
			// build out cli context
			// debug flag
			ctx := context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, viper.GetBool("debug"))
			ctx = context.WithValue(ctx, appctx.PaymentsServiceCTXKey, viper.GetString("payments-service"))
			ctx = context.WithValue(ctx, appctx.CACertCTXKey, viper.GetString("ca-cert"))
			ctx = context.WithValue(ctx, appctx.CustodianCTXKey, viper.GetString("custodian"))
			ctx = context.WithValue(ctx, appctx.PayoutFileLocationCTXKey, viper.GetString("payout-file"))
			ctx = context.WithValue(ctx, appctx.KeyPairFileLocationCTXKey, viper.GetString("key-pair"))
			ctx = context.WithValue(ctx, appctx.PaymentsDocumentIDCTXKey, viper.GetString("document-id"))

			// setup logger
			ctx, logger := logging.SetupLogger(ctx)
			logger.Info().
				Str("debug", viper.GetString("debug")).
				Str("payments_service", viper.GetString("payments-service")).
				Msg("logger setup")

			// add in subcommands (context enabled)
			cmd.AddCommand(prepareCmd(ctx))
			cmd.AddCommand(authorizeCmd(ctx))
			cmd.AddCommand(submitCmd(ctx))

			// validate args
			found := false
			if len(args) < 1 {
				logger.Error().Msg("subcommand required: prepare, authorize, submit")
				return
			}
			for _, v := range []string{
				"prepare", "authorize", "submit",
			} {
				if args[0] == v {
					found = true
				}
			}
			if !found {
				logger.Error().Msg("subcommand invalid, should be in: prepare, authorize, submit")
				return
			}
			// proceed with execution of current command
			err := cmd.ExecuteContext(ctx)
			if err != nil {
				logger.Error().Err(err).Msg("failed execution")
				return
			}
		},
	}
)
