package cmd

import (
	"context"
	"fmt"

	cmdutils "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/tools/nitro-settlement/internal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	// add validate to our root settlement-cli command
	rootCmd.AddCommand(validateCmd)

	// configurations for validate command

	// report as input
	validateCmd.Flags().String(reportKey, "", "report input for validate")
	viper.BindPFlag(validateReportKey, validateCmd.Flags().Lookup(reportKey))

	// report as input, this is the validated report attested
	validateCmd.Flags().String(attestedReportKey, "", "attested report input for validate")
	viper.BindPFlag(validateAttestedReportKey, validateCmd.Flags().Lookup(attestedReportKey))

	// payments service host -- to get the ed25519 ephemeral signing key
	validateCmd.Flags().String(paymentsHostKey, "", "the payments service host")
	viper.BindPFlag(paymentsHostKey, validateCmd.Flags().Lookup(paymentsHostKey))
}

// validateCmd is the nitro settlements prepare command, which loads transactions into workflow
var (
	validateCmd = &cobra.Command{
		Use:   "validate",
		Short: "validate transactions for settlement",
		Run:   cmdutils.Perform("validate settlement", validateRun),
	}
	validateReportKey         = "validate-report"
	validateAttestedReportKey = "validate-attested-report"
)

// validateRun - main entrypoint for the `validate` subcommand
func validateRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "validate").Info().Msg("performing validate...")

	logging.Logger(ctx, "validate").Info().
		Str(attestedReportKey, viper.GetString(validateAttestedReportKey)).
		Str(reportKey, viper.GetString(validateReportKey)).
		Str(paymentsHostKey, viper.GetString(paymentsHostKey)).
		Msg("configuration")

	// get the payments service's signing key
	paymentsPubKey, err := internal.GetPaymentsPubKey(ctx, viper.GetString(paymentsHostKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "validate", "failed to download payments public key")
	}
	logging.Logger(ctx, "validate").Info().Msg("payments public key downloaded...")

	// read the original report
	originalReport, err := internal.ParseReport(ctx, viper.GetString(validateReportKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "validate", "failed to parse original report")
	}
	logging.Logger(ctx, "validate").Info().Msg("original report has been parsed...")

	// read the attested report
	attestedReport, err := internal.ParseAttestedReport(ctx, viper.GetString(validateAttestedReportKey))
	if err != nil {
		return internal.LogAndError(ctx, err, "validate", "failed to parse attested report")
	}
	logging.Logger(ctx, "validate").Info().Msg("attested report has been parsed...")

	if originalReport.Length() != attestedReport.Length() {
		return internal.LogAndError(
			ctx, err, "validate",
			fmt.Sprintf(
				"transaction count mismatch, original %d != attested %d",
				originalReport.Length(),
				attestedReport.Length()))
	}

	if !viper.GetBool(testModeKey) {
		if !originalReport.SumBAT().Equal(attestedReport.SumBAT()) {
			return internal.LogAndError(
				ctx, err, "validate",
				fmt.Sprintf(
					"transaction sum mismatch, original %s != attested %s",
					originalReport.SumBAT(),
					attestedReport.SumBAT()))
		}
	}

	// check signatures on each transaction in attested file with paymentsPubKey
	if err := attestedReport.Verify(ctx, paymentsPubKey); err != nil {
		return internal.LogAndError(ctx, err, "validate", "failed to verify attested report with signing key")
	}
	logging.Logger(ctx, "validate").Info().Msg("attested report has been verified...")

	logging.Logger(ctx, "validate").Info().
		Str("count", fmt.Sprintf("%d", originalReport.Length())).
		Str("total", originalReport.SumBAT().String()).
		Str("gemini", originalReport.SumBAT(internal.GeminiCustodian).String()).
		Str("uphold", originalReport.SumBAT(internal.UpholdCustodian).String()).
		Str("bitflyer", originalReport.SumBAT(internal.BitflyerCustodian).String()).
		Msg("completed validation.")

	return nil
}
