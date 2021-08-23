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

	// pprof-enabled - defaults to ""
	paymentsCliCmd.PersistentFlags().String("payments-service", "",
		"the location of the payments service")
	cmd.Must(viper.BindPFlag("payments-service", paymentsCliCmd.PersistentFlags().Lookup("payments-service")))
	cmd.Must(viper.BindEnv("payments-service", "PAYMENTS_SERVICE"))

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
			ctx = context.WithValue(context.Background(), appctx.PaymentsServiceCTXKey, viper.GetString("payments-service"))
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
