package settlement

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/settlement"
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

// GetBitflyerToken gets a new bitflyer token from cobra command
func GetBitflyerToken(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	v := viper.GetViper()
	clientID := v.GetString("bitflyer-client-id")
	clientSecret := v.GetString("bitflyer-client-secret")
	extraClientSecret := v.GetString("bitflyer-extra-client-secret")
	client, err := bitflyer.New()
	if err != nil {
		return err
	}
	payload := bitflyer.TokenPayload{
		GrantType:         "client_credentials",
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		ExtraClientSecret: extraClientSecret,
	}
	auth, err := client.RefreshToken(
		ctx,
		payload,
	)
	if err != nil {
		return err
	}
	logger.Info().Interface("auth", auth).
		Msg("token refreshed")
	return nil
}

// UploadBitflyerSettlement uploads bitflyer settlement
func UploadBitflyerSettlement(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	input := v.GetString("input")
	out := v.GetString("out")
	token := v.GetString("token")
	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + "-finished.json"
	}
	return BitflyerUploadSettlement(
		cmd.Context(),
		"upload",
		input,
		out,
		token,
	)
}

// CheckStatusBitflyerSettlement is the command runner for checking bitflyer transactions status
func CheckStatusBitflyerSettlement(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	input := v.GetString("input")
	out := v.GetString("out")
	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + "-finished.json"
	}
	token := v.GetString("token")
	return BitflyerUploadSettlement(
		cmd.Context(),
		"checkstatus",
		input,
		out,
		token,
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
		Bind("input").
		Env("INPUT")

	uploadCheckStatusBuilder.Flag().String("out", "./bitflyer-settlement",
		"the location of the file").
		Bind("out").
		Env("OUT")

	uploadCheckStatusBuilder.Flag().String("bitflyer-source-from", "self",
		"tells bitflyer where to draw funds from").
		Bind("bitflyer-source-from").
		Env("BITFLYER_SOURCE_FROM")

	uploadCheckStatusBuilder.Flag().Bool("bitflyer-dry-run", false,
		"tells bitflyer that this is a practice round").
		Bind("bitflyer-dry-run").
		Env("BITFLYER_DRYRUN")

	uploadCheckStatusBuilder.Flag().String("bitflyer-client-token", "",
		"the token to be sent for auth on bitflyer").
		Bind("bitflyer-client-token").
		Env("BITFLYER_CLIENT_TOKEN")

	tokenBuilder.Flag().String("bitflyer-client-id", "",
		"tells bitflyer what the client id is during token generation").
		Bind("bitflyer-client-id").
		Env("BITFLYER_CLIENT_ID")

	tokenBuilder.Flag().String("bitflyer-client-secret", "",
		"tells bitflyer what the client secret during token generation").
		Bind("bitflyer-client-secret").
		Env("BITFLYER_CLIENT_SECRET")

	tokenBuilder.Flag().String("bitflyer-extra-client-secret", "",
		"tells bitflyer what the extra client secret is during token generation").
		Bind("bitflyer-extra-client-secret").
		Env("BITFLYER_EXTRA_CLIENT_SECRET")

	allBuilder.Flag().String("bitflyer-server", "",
		"the bitflyer domain to interact with").
		Bind("bitflyer-server").
		Env("BITFLYER_SERVER")
}

// BitflyerUploadSettlement marks the settlement file as complete
func BitflyerUploadSettlement(ctx context.Context, action string, inPath string, outPath string, token string) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	fmt.Println("action", action)
	fmt.Println("inpath", inPath)
	fmt.Println("outPath", outPath)
	fmt.Println("token", token)
	if outPath == "./bitflyer-settlement" {
		// use a file with extension if none is passed
		outPath = "./bitflyer-settlement-complete.json"
	}

	bulkPayoutFiles := strings.Split(inPath, ",")
	bitflyerClient, err := bitflyer.New()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create new bitflyer client")
		return err
	}
	// set the auth token
	bitflyerClient.SetAuthToken(token)

	submittedTransactions, submitErr := bitflyersettlement.IterateRequest(
		ctx,
		action,
		bitflyerClient,
		bulkPayoutFiles,
	)
	// write file for upload to eyeshade
	logger.Info().
		Str("files", outPath).
		Msg("outputting files")

	if submittedTransactions != nil {
		for key, txs := range *submittedTransactions {
			if len(txs) > 0 {
				outputPath := strings.TrimSuffix(outPath, filepath.Ext(outPath)) + "-" + key + ".json"
				err = BitflyerWriteTransactions(ctx, outputPath, &txs)
				if err != nil {
					logger.Error().Err(err).Msg("failed to write bitflyer transactions file")
					return err
				}
			}
		}
	}
	return submitErr
}

// BitflyerWriteTransactions writes settlement transactions to a json file
func BitflyerWriteTransactions(ctx context.Context, outPath string, metadata *[]settlement.Transaction) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}

	if len(*metadata) == 0 {
		return nil
	}

	logger.Debug().Str("files", outPath).Int("num transactions", len(*metadata)).Msg("writing outputting files")
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		logger.Error().Err(err).Msg("failed writing outputting files")
		return err
	}
	return ioutil.WriteFile(outPath, data, 0600)
}
