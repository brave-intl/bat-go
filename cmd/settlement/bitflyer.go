package settlement

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/cmd"
	bitflyersettlement "github.com/brave-intl/bat-go/settlement/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// BitflyerSettlementCmd creates the bitflyer subcommand
	BitflyerSettlementCmd = &cobra.Command{
		Use:   "bitflyer",
		Short: "facilitates bitflyer settlement",
	}

	// UploadBitflyerSettlementCmd creates the bitflyer uphold subcommand
	UploadBitflyerSettlementCmd = &cobra.Command{
		Use:   "upload",
		Short: "uploads signed bitflyer transactions",
		Run:   cmd.Perform("bitflyer upload", UploadBitflyerSettlement),
	}

	// CheckStatusBitflyerSettlementCmd creates the bitflyer checkstatus subcommand
	CheckStatusBitflyerSettlementCmd = &cobra.Command{
		Use:   "checkstatus",
		Short: "uploads signed bitflyer transactions",
		Run:   cmd.Perform("bitflyer checkstatus", CheckStatusBitflyerSettlement),
	}

	// GetBitflyerTokenCmd gets a new bitflyer token
	GetBitflyerTokenCmd = &cobra.Command{
		Use:   "token",
		Short: "gets a new token for authing",
		Run:   cmd.Perform("bitflyer token", GetBitflyerToken),
	}
)

// NewRefreshTokenPayloadFromViper creates the payload to refresh a bitflyer token from viper
func NewRefreshTokenPayloadFromViper() *bitflyer.TokenPayload {
	vpr := viper.GetViper()
	clientID := vpr.GetString("bitflyer-client-id")
	clientSecret := vpr.GetString("bitflyer-client-secret")
	extraClientSecret := vpr.GetString("bitflyer-extra-client-secret")
	return &bitflyer.TokenPayload{
		GrantType:         "client_credentials",
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		ExtraClientSecret: extraClientSecret,
	}
}

// GetBitflyerToken gets a new bitflyer token from cobra command
func GetBitflyerToken(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	refreshTokenPayload := NewRefreshTokenPayloadFromViper()
	client, err := bitflyer.New()
	if err != nil {
		return err
	}
	auth, err := client.RefreshToken(
		ctx,
		*refreshTokenPayload,
	)
	if err != nil {
		return err
	}
	logger.Info().
		Str("access_token", auth.AccessToken).
		Int("expires_in", auth.ExpiresIn).
		Str("refresh_token", auth.RefreshToken).
		Str("scope", auth.Scope).
		Str("token_type", auth.TokenType).
		Msg("token refreshed")
	return nil
}

// UploadBitflyerSettlement uploads bitflyer settlement
func UploadBitflyerSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	token := viper.GetViper().GetString("bitflyer-client-token")
	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + "-finished.json"
	}
	sourceFrom, err := cmd.Flags().GetString("bitflyer-source-from")
	if err != nil {
		return err
	}
	excludeLimited, err := cmd.Flags().GetBool("exclude-limited")
	if err != nil {
		return err
	}
	dryRunOptions, err := ParseDryRun(cmd)
	if err != nil {
		return err
	}
	return BitflyerUploadSettlement(
		cmd.Context(),
		"upload",
		input,
		out,
		token,
		sourceFrom,
		excludeLimited,
		dryRunOptions,
	)
}

// ParseDryRun parses the dry run option
func ParseDryRun(cmd *cobra.Command) (*bitflyer.DryRunOption, error) {
	dryRun, err := cmd.Flags().GetBool("bitflyer-dryrun")
	if err != nil {
		return nil, err
	}
	var dryRunOptions *bitflyer.DryRunOption
	if dryRun {
		dryRunDuration, err := cmd.Flags().GetDuration("bitflyer-process-time")
		if err != nil {
			return nil, err
		}
		dryRunOptions = &bitflyer.DryRunOption{
			ProcessTimeSec: uint(dryRunDuration.Seconds()),
		}
	}
	return dryRunOptions, nil
}

// CheckStatusBitflyerSettlement is the command runner for checking bitflyer transactions status
func CheckStatusBitflyerSettlement(cmd *cobra.Command, args []string) error {
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + "-finished.json"
	}
	token := viper.GetViper().GetString("bitflyer-client-token")
	excludeLimited, err := cmd.Flags().GetBool("exclude-limited")
	if err != nil {
		return err
	}
	sourceFrom, err := cmd.Flags().GetString("bitflyer-source-from")
	if err != nil {
		return err
	}
	dryRunOptions, err := ParseDryRun(cmd)
	if err != nil {
		return err
	}
	return BitflyerUploadSettlement(
		cmd.Context(),
		"checkstatus",
		input,
		out,
		token,
		sourceFrom,
		excludeLimited,
		dryRunOptions,
	)
}

func init() {
	// add complete and transform subcommand
	BitflyerSettlementCmd.AddCommand(GetBitflyerTokenCmd)
	BitflyerSettlementCmd.AddCommand(UploadBitflyerSettlementCmd)
	BitflyerSettlementCmd.AddCommand(CheckStatusBitflyerSettlementCmd)

	// add this command as a settlement subcommand
	SettlementCmd.AddCommand(BitflyerSettlementCmd)

	// setup the flags
	tokenBuilder := cmd.NewFlagBuilder(GetBitflyerTokenCmd)
	uploadCheckStatusBuilder := cmd.NewFlagBuilder(UploadBitflyerSettlementCmd).
		AddCommand(CheckStatusBitflyerSettlementCmd)
	allBuilder := tokenBuilder.Concat(uploadCheckStatusBuilder)

	uploadCheckStatusBuilder.Flag().String("input", "",
		"the file or comma delimited list of files that should be utilized. both referrals and contributions should be done in one command in order to group the transactions appropriately").
		Require().
		Bind("input")

	uploadCheckStatusBuilder.Flag().String("out", "./bitflyer-settlement",
		"the location of the file").
		Bind("out").
		Env("OUT")

	uploadCheckStatusBuilder.Flag().String("bitflyer-source-from", "tipping",
		"tells bitflyer where to draw funds from").
		Bind("bitflyer-source-from").
		Env("BITFLYER_SOURCE_FROM")

	uploadCheckStatusBuilder.Flag().Bool("bitflyer-dryrun", false,
		"tells bitflyer that this is a practice round").
		Bind("bitflyer-dryrun").
		Env("BITFLYER_DRYRUN")

	uploadCheckStatusBuilder.Flag().Duration("bitflyer-process-time", time.Second,
		"tells bitflyer the duration of this practice round").
		Bind("bitflyer-dryrun").
		Env("BITFLYER_DRYRUN")

	uploadCheckStatusBuilder.Flag().String("bitflyer-client-token", "",
		"the token to be sent for auth on bitflyer").
		Bind("bitflyer-client-token").
		Env("BITFLYER_TOKEN")

	tokenBuilder.Flag().String("bitflyer-client-id", "",
		"tells bitflyer what the client id is during token generation").
		Bind("bitflyer-client-id").
		Env("BITFLYER_CLIENT_ID")

	tokenBuilder.Flag().String("bitflyer-client-secret", "",
		"tells bitflyer what the client secret during token generation").
		Bind("bitflyer-client-secret").
		Env("BITFLYER_CLIENT_SECRET")

	tokenBuilder.Flag().String("bitflyer-extra-client-secret", "",
		"tells bitflyer what the extra client secret is during token").
		Bind("bitflyer-extra-client-secret").
		Env("BITFLYER_EXTRA_CLIENT_SECRET")

	allBuilder.Flag().String("bitflyer-server", "",
		"the bitflyer domain to interact with").
		Bind("bitflyer-server").
		Env("BITFLYER_SERVER")

	allBuilder.Flag().Bool("exclude-limited", false,
		"in order to avoid not knowing what the payout amount will be because of transfer limits").
		Bind("exclude-limited")
}

// BitflyerUploadSettlement marks the settlement file as complete
func BitflyerUploadSettlement(
	ctx context.Context,
	action, inPath, outPath, token, sourceFrom string,
	excludeLimited bool,
	dryRun *bitflyer.DryRunOption,
) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	bulkPayoutFiles := strings.Split(inPath, ",")
	bitflyerClient, err := bitflyer.New()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create new bitflyer client")
		return err
	}
	// set the auth token
	if token != "" {
		bitflyerClient.SetAuthToken(token)
	} else {
		refreshTokenPayload := NewRefreshTokenPayloadFromViper()
		_, err := bitflyerClient.RefreshToken(ctx, *refreshTokenPayload)
		if err != nil {
			return err
		}
	}

	submittedTransactions, submitErr := bitflyersettlement.IterateRequest(
		ctx,
		action,
		bitflyerClient,
		bulkPayoutFiles,
		sourceFrom,
		excludeLimited,
		dryRun,
	)
	// write file for upload to eyeshade
	logger.Info().
		Str("files", outPath).
		Msg("outputting files")

	err = WriteCategorizedTransactions(ctx, outPath, submittedTransactions)
	if err != nil {
		logger.Error().Err(err).Msg("failed to write transactions file")
		return err
	}
	return submitErr
}
