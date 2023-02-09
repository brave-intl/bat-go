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

	// root nitro certificate for attestation verification
	validateCmd.Flags().String(rootNitroCertFilenameKey, "", "the nitro root cert file")
	viper.BindPFlag(rootNitroCertFilenameKey, validateCmd.Flags().Lookup(rootNitroCertFilenameKey))
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
	rootNitroCertFilenameKey  = "nitro-cert"
)

// validateRun - main entrypoint for the `validate` subcommand
func validateRun(command *cobra.Command, args []string) error {
	ctx := context.WithValue(command.Context(), internal.TestModeCTXKey, viper.GetBool(testModeKey))
	logging.Logger(ctx, "validate").Info().Msg("performing validate...")

	logging.Logger(ctx, "validate").Info().
		Str(attestedReportKey, viper.GetString(validateAttestedReportKey)).
		Str(reportKey, viper.GetString(validateReportKey)).
		Str(rootNitroCertFilenameKey, viper.GetString(rootNitroCertFilenameKey)).
		Msg("configuration")

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

	if !originalReport.SumBAT().Equal(attestedReport.SumBAT()) {
		return internal.LogAndError(
			ctx, err, "validate",
			fmt.Sprintf(
				"transaction sum mismatch, original %s != attested %s",
				originalReport.SumBAT(),
				attestedReport.SumBAT()))
	}

	// check nitro attestation document on each transaction in attested file
	if err := attestedReport.Verify(ctx, viper.GetString(rootNitroCertFilenameKey)); err != nil {
		return internal.LogAndError(ctx, err, "validate", "failed to verify attested report")
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
