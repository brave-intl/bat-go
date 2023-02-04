package cmd

import (
	"context"
	"strings"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/nitro-settlement/internal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add authorize to our root settlement-cli command
	rootCmd.AddCommand(authorizeCmd)

	// configurations for authorize command

	// report as input, this is the validated report attested
	authorizeCmd.Flags().String(attestedReportKey, "", "attested report input for authorize")
	viper.BindPFlag(authorizeAttestedReportKey, authorizeCmd.Flags().Lookup(attestedReportKey))

	// payout identifier as input
	authorizeCmd.Flags().String(payoutIDKey, "", "the identifier of the payout (20230202 for example)")
	viper.BindPFlag(payoutIDKey, authorizeCmd.Flags().Lookup(payoutIDKey))

	// operator private key file as input
	authorizeCmd.Flags().String(privateKeyFileKey, "", "the private key file")
	viper.BindPFlag(privateKeyFileKey, authorizeCmd.Flags().Lookup(privateKeyFileKey))

	// payments service host
	authorizeCmd.Flags().String(paymentsHostKey, "", "the payments service host")
	viper.BindPFlag(paymentsHostKey, authorizeCmd.Flags().Lookup(paymentsHostKey))
}

// authorizeCmd is the nitro settlements prepare command, which loads transactions into workflow
var (
	authorizeCmd = &cobra.Command{
		Use:   "authorize",
		Short: "authorize transactions for settlement",
		Run:   cmdutils.Perform("authorize settlement", authorizeRun),
	}
	privateKeyFileKey          = "key-file"
	attestedReportKey          = "attested-report"
	authorizeAttestedReportKey = "authorize-attested-report"
	paymentsHostKey            = "payments-host"
)

// authorizeRun - main entrypoint for the `authorize` subcommand
func authorizeRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "authorize").Info().Msg("performing authorize...")

	logging.Logger(ctx, "authorize").Info().
		Str(attestedReportKey, viper.GetString(authorizeAttestedReportKey)).
		Str(payoutIDKey, viper.GetString(payoutIDKey)).
		Str(privateKeyFileKey, viper.GetString(privateKeyFileKey)).
		Str(redisAddrKey, strings.Join(viper.GetStringSlice(redisAddrKey), ", ")).
		Str(redisUserKey, viper.GetString(redisUserKey)).
		Str(redisPassKey, viper.GetString(redisPassKey)).
		Msg("configuration")

	publisher, err := internal.GetPublisher(ctx, viper.GetStringSlice(redisAddrKey), viper.GetString(redisUserKey), viper.GetString(redisPassKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "authorize", "failed to setup publisher")
	}
	logging.Logger(ctx, "authorize").Info().Msg("created publisher...")

	// read the report
	attestedReport, err := internal.ParseAttestedReport(ctx, viper.GetString(authorizeAttestedReportKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "authorize", "failed to parse attested report")
	}
	logging.Logger(ctx, "authorize").Info().Msg("attested report has been parsed...")

	operatorKey, err := internal.GetOperatorPrivateKey(ctx, viper.GetString(privateKeyFileKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "authorize", "failed to parse attested report")
	}
	logging.Logger(ctx, "authorize").Info().Msg("loaded operator key...")

	// publish signed transactions
	stream, records, err := publisher.SignAndPublishTransactions(
		ctx, viper.GetString(payoutIDKey), attestedReport,
		viper.GetString(paymentsHostKey), operatorKey,
	)
	if err != nil {
		return internal.LogAndError(ctx, err, "attested", "failed to publish signed transactions")
	}
	logging.Logger(ctx, "prepare").Info().Msg("signed transactions have been published...")

	// inform settlement workers
	if err := publisher.ConfigureWorker(ctx, internal.AuthorizeConfigStream, &internal.WorkerConfig{
		PayoutID: viper.GetString(payoutIDKey),
		Stream:   stream,
		Count:    records,
	}); err != nil {
		return internal.LogAndError(ctx, err, "authorize", "failed to configure authorize worker")
	}
	logging.Logger(ctx, "authorize").Info().Msg("settlement workers have been configured...")

	logging.Logger(ctx, "authorize").Info().Msg("completed authorize.")
	return nil
}
