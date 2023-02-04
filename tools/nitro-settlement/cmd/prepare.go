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
	// add prepare to our root settlement-cli command
	rootCmd.AddCommand(prepareCmd)

	// configurations for prepare command

	// report as input
	prepareCmd.Flags().String(reportKey, "", "report input for prepare")
	viper.BindPFlag(prepareReportKey, prepareCmd.Flags().Lookup(reportKey))

	// payout identifier as input
	prepareCmd.Flags().String(payoutIDKey, "", "the identifier of the payout (20230202 for example)")
	viper.BindPFlag(payoutIDKey, prepareCmd.Flags().Lookup(payoutIDKey))
}

// prepareCmd is the nitro settlements prepare command, which loads transactions into workflow
var (
	prepareCmd = &cobra.Command{
		Use:   "prepare",
		Short: "prepare transactions for settlement",
		Run:   cmdutils.Perform("prepare settlement", prepareRun),
	}
	payoutIDKey      = "payout-id"
	reportKey        = "report"
	prepareReportKey = "prepare-report"
)

// prepareRun - main entrypoint for the `prepare` subcommand
func prepareRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "prepare").Info().Msg("performing prepare...")

	logging.Logger(ctx, "prepare").Info().
		Str(reportKey, viper.GetString(prepareReportKey)).
		Str(payoutIDKey, viper.GetString(payoutIDKey)).
		Str(redisAddrKey, strings.Join(viper.GetStringSlice(redisAddrKey), ", ")).
		Str(redisUserKey, viper.GetString(redisUserKey)).
		Str(redisPassKey, viper.GetString(redisPassKey)).
		Msg("configuration")

	publisher, err := internal.GetPublisher(ctx, viper.GetStringSlice(redisAddrKey), viper.GetString(redisUserKey), viper.GetString(redisPassKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "prepare", "failed to setup publisher")
	}
	logging.Logger(ctx, "prepare").Info().Msg("created publisher...")

	// read the report
	report, err := internal.ParseReport(ctx, viper.GetString(prepareReportKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "prepare", "failed to prepare report")
	}
	logging.Logger(ctx, "prepare").Info().Msg("report has been parsed...")

	// publish transactions
	stream, records, err := publisher.PublishReport(ctx, viper.GetString(payoutIDKey), report)
	if err != nil {
		return internal.LogAndError(ctx, err, "prepare", "failed to publish report")
	}
	logging.Logger(ctx, "prepare").Info().Msg("report has been published...")

	// inform settlement workers
	if err := publisher.ConfigureWorker(ctx, internal.PrepareConfigStream, &internal.WorkerConfig{
		PayoutID: viper.GetString(payoutIDKey),
		Count:    records,
		Stream:   stream,
	}); err != nil {
		return internal.LogAndError(ctx, err, "prepare", "failed to configure prepare worker")
	}
	logging.Logger(ctx, "prepare").Info().Msg("settlement workers have been configured...")

	logging.Logger(ctx, "prepare").Info().Msg("completed prepare.")
	return nil
}
